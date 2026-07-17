package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"wallpaper-chooser/internal/database"
)

type Progress struct {
	WallpaperID int64  `json:"wallpaperId"`
	Status      string `json:"status"`
	Progress    int    `json:"progress"`
	Error       string `json:"error,omitempty"`
}

type Downloader struct {
	db          *database.DB
	cacheDir    string
	thumbDir    string
	concurrency int
	client      *http.Client
	thumbClient *http.Client
	mu          sync.Mutex
	active      int32
	queue       chan *downloadJob
	progress    chan Progress
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	closed      int32
	cancels     map[int64]context.CancelFunc
	cancelsMu   sync.Mutex
}

type downloadJob struct {
	wallpaper database.Wallpaper
	priority  int
	ctx       context.Context
}

func New(db *database.DB, cacheDir, thumbDir string, concurrency int) *Downloader {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Downloader{
		db:          db,
		cacheDir:    cacheDir,
		thumbDir:    thumbDir,
		concurrency: concurrency,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
		thumbClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		queue:    make(chan *downloadJob, 5000),
		progress: make(chan Progress, 500),
		ctx:      ctx,
		cancel:   cancel,
		cancels:  make(map[int64]context.CancelFunc),
	}

	for i := 0; i < concurrency; i++ {
		d.wg.Add(1)
		go d.worker(i)
	}

	return d
}

func (d *Downloader) Start() {}

func (d *Downloader) Stop() {
	atomic.StoreInt32(&d.closed, 1)
	d.cancel()
	close(d.queue)
	d.wg.Wait()
	close(d.progress)
}

func (d *Downloader) ProgressChan() <-chan Progress {
	return d.progress
}

func (d *Downloader) QueueCount() int {
	return len(d.queue)
}

func (d *Downloader) ActiveCount() int {
	return int(atomic.LoadInt32(&d.active))
}

func (d *Downloader) EnqueueThumbnail(w database.Wallpaper) {
	if atomic.LoadInt32(&d.closed) == 1 {
		return
	}
	select {
	case d.queue <- &downloadJob{wallpaper: w, priority: 0, ctx: d.ctx}:
	default:
	}
}

func (d *Downloader) EnqueueFull(w database.Wallpaper) {
	if atomic.LoadInt32(&d.closed) == 1 {
		return
	}

	d.cancelsMu.Lock()
	if _, exists := d.cancels[w.ID]; exists {
		d.cancelsMu.Unlock()
		return
	}

	dlCtx, dlCancel := context.WithCancel(d.ctx)
	d.cancels[w.ID] = dlCancel
	d.cancelsMu.Unlock()

	select {
	case d.queue <- &downloadJob{wallpaper: w, priority: 1, ctx: dlCtx}:
	default:
		d.cancelsMu.Lock()
		delete(d.cancels, w.ID)
		d.cancelsMu.Unlock()
		dlCancel()
		log.Printf("[downloader] queue full, dropping wallpaper %d", w.ID)
	}
}

func (d *Downloader) CancelDownload(id int64) {
	d.cancelsMu.Lock()
	if cancel, ok := d.cancels[id]; ok {
		cancel()
		delete(d.cancels, id)
	}
	d.cancelsMu.Unlock()

	d.db.UpdateStatus(id, "scraped")
	d.progress <- Progress{WallpaperID: id, Status: "cancelled"}
}

func (d *Downloader) EnqueueMany(wallpapers []database.Wallpaper) {
	for _, w := range wallpapers {
		d.EnqueueThumbnail(w)
	}
}

func (d *Downloader) worker(id int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[downloader] worker %d recovered from panic: %v", id, r)
		}
		d.wg.Done()
	}()

	for job := range d.queue {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		atomic.AddInt32(&d.active, 1)
		if job.priority == 0 {
			d.downloadThumbnail(job)
		} else {
			d.downloadFull(job)
		}
		atomic.AddInt32(&d.active, -1)
	}
}

func (d *Downloader) downloadThumbnail(job *downloadJob) {
	w := job.wallpaper
	if w.ThumbnailURL == "" {
		return
	}

	ext := filepath.Ext(w.ThumbnailURL)
	if ext == "" || len(ext) > 5 {
		ext = ".jpg"
	}
	thumbPath := filepath.Join(d.thumbDir, fmt.Sprintf("%d%s", w.ID, ext))

	if err := d.downloadFileHTTPContext(job.ctx, d.thumbClient, w.ThumbnailURL, thumbPath); err != nil {
		if job.ctx.Err() != nil {
			log.Printf("[downloader] thumb cancelled %d", w.ID)
			os.Remove(thumbPath)
			return
		}
		log.Printf("[downloader] thumb failed %d: %v", w.ID, err)
		d.db.IncrementThumbAttempts(w.ID)
		return
	}

	info, err := os.Stat(thumbPath)
	if err != nil || info.Size() < 1024 {
		log.Printf("[downloader] thumb too small %d (%d bytes)", w.ID, info.Size())
		os.Remove(thumbPath)
		d.db.IncrementThumbAttempts(w.ID)
		return
	}

	d.db.UpdateThumbnailPath(w.ID, thumbPath)
	logf("thumbnail %d saved (%d bytes)", w.ID, info.Size())
}

func (d *Downloader) downloadFull(job *downloadJob) {
	w := job.wallpaper
	logf("downloading full %s (id=%d)", w.URL, w.ID)

	d.db.UpdateStatus(w.ID, "downloading")
	d.progress <- Progress{WallpaperID: w.ID, Status: "downloading"}

	ext := filepath.Ext(w.URL)
	if ext == "" || len(ext) > 5 {
		ext = ".jpg"
	}
	filename := fmt.Sprintf("%d%s", w.ID, ext)
	localPath := filepath.Join(d.cacheDir, filename)

	if err := d.downloadFileHTTPContext(job.ctx, d.client, w.URL, localPath); err != nil {
		if job.ctx.Err() != nil {
			log.Printf("[downloader] cancelled %d", w.ID)
			os.Remove(localPath)
			d.db.UpdateStatus(w.ID, "scraped")
			d.cancelsMu.Lock()
			delete(d.cancels, w.ID)
			d.cancelsMu.Unlock()
			return
		}
		log.Printf("[downloader] failed %s: %v", w.URL, err)
		d.db.UpdateStatus(w.ID, "failed")
		d.progress <- Progress{WallpaperID: w.ID, Status: "failed", Error: err.Error()}
		d.cancelsMu.Lock()
		delete(d.cancels, w.ID)
		d.cancelsMu.Unlock()
		return
	}

	if job.ctx.Err() != nil {
		log.Printf("[downloader] cancelled after download %d", w.ID)
		os.Remove(localPath)
		d.db.UpdateStatus(w.ID, "scraped")
		d.cancelsMu.Lock()
		delete(d.cancels, w.ID)
		d.cancelsMu.Unlock()
		return
	}

	hash, err := fileHash(localPath)
	if err != nil {
		log.Printf("[downloader] hash error: %v", err)
	} else {
		if d.db.ExistsByHash(hash) {
			log.Printf("[downloader] duplicate hash %s for %d", hash, w.ID)
			os.Remove(localPath)
			d.db.UpdateStatus(w.ID, "duplicate")
			d.progress <- Progress{WallpaperID: w.ID, Status: "duplicate"}
			d.cancelsMu.Lock()
			delete(d.cancels, w.ID)
			d.cancelsMu.Unlock()
			return
		}
		d.db.UpdateHash(w.ID, hash)
	}

	d.db.UpdateLocalPath(w.ID, localPath)
	d.progress <- Progress{WallpaperID: w.ID, Status: "downloaded"}

	d.cancelsMu.Lock()
	delete(d.cancels, w.ID)
	d.cancelsMu.Unlock()

	info, _ := os.Stat(localPath)
	if info != nil {
		logf("downloaded %s (%d bytes)", w.URL, info.Size())
	}
}

func (d *Downloader) downloadFileHTTPContext(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func logf(format string, args ...interface{}) {
	log.Printf("[downloader] "+format, args...)
}
