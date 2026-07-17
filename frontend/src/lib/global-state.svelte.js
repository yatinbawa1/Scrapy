export const appState = $state({
  view: 'grid',
  previewOpen: false,
  isFirstRun: true,
  isScraping: false,
  isLoading: false,
  wallpapers: [],
  currentWallpaper: null,
  stats: {},
  downloadQueue: { active: 0, pending: 0 },
  favoritesOnly: false,
  currentPage: 1,
  totalResults: 0,
  sortBy: 'random',
  category: '',
  scrapeProgress: { source: '', message: '' },
  downloads: [],
  showOnboarding: true,
  cacheDir: '',
  favoritesCount: 0,
  categoriesStats: [],
  sourceUsage: [],
});

export function downloadStart(id, origin) {
  const existing = appState.downloads.find(d => d.id === id);
  if (existing) {
    appState.downloads = appState.downloads.map(d =>
      d.id === id ? { ...d, origin, status: 'downloading' } : d
    );
  } else {
    appState.downloads = [...appState.downloads, { id, origin, status: 'downloading', ts: Date.now() }];
  }
}

export function downloadDone(id) {
  appState.downloads = appState.downloads.map(d =>
    d.id === id ? { ...d, status: 'done' } : d
  );
}

export function downloadFail(id, error) {
  appState.downloads = appState.downloads.map(d =>
    d.id === id ? { ...d, status: 'failed', error } : d
  );
}

export function downloadRemove(id) {
  appState.downloads = appState.downloads.filter(d => d.id !== id);
}

export function downloadGetOrigin(id) {
  const item = appState.downloads.find(d => d.id === id);
  return item ? item.origin : null;
}

export function downloadIsActive(id) {
  const item = appState.downloads.find(d => d.id === id);
  return item && item.status === 'downloading';
}

export function downloadClear() {
  appState.downloads = [];
}

export function getActiveDownloads() {
  return appState.downloads.filter(d => d.status === 'downloading');
}

export function getCompletedDownloads() {
  return appState.downloads.filter(d => d.status === 'done');
}
