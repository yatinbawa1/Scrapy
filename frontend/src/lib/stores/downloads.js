import { writable, derived, get } from 'svelte/store';

function createDownloadsStore() {
  const { subscribe, update, set } = writable([]);

  return {
    subscribe,

    start(id, origin) {
      update(list => {
        const existing = list.find(d => d.id === id);
        if (existing) {
          return list.map(d => d.id === id ? { ...d, origin, status: 'downloading' } : d);
        }
        return [...list, { id, origin, status: 'downloading', ts: Date.now() }];
      });
    },

    done(id) {
      update(list =>
        list.map(d => d.id === id ? { ...d, status: 'done' } : d)
      );
    },

    fail(id, error) {
      update(list =>
        list.map(d => d.id === id ? { ...d, status: 'failed', error } : d)
      );
    },

    remove(id) {
      update(list => list.filter(d => d.id !== id));
    },

    getOrigin(id) {
      const item = get({ subscribe }).find(d => d.id === id);
      return item ? item.origin : null;
    },

    isActive(id) {
      const item = get({ subscribe }).find(d => d.id === id);
      return item && item.status === 'downloading';
    },

    clear() {
      set([]);
    }
  };
}

export const downloads = createDownloadsStore();

export const activeDownloads = derived(downloads, $d =>
  $d.filter(d => d.status === 'downloading')
);

export const completedDownloads = derived(downloads, $d =>
  $d.filter(d => d.status === 'done')
);

export const activeCount = derived(activeDownloads, $d => $d.length);
