import { writable } from 'svelte/store';

function logged(name, initial) {
  const store = writable(initial);
  store.subscribe(v => console.log(`[store:${name}] =`, v));
  return store;
}

export const wallpapers = logged('wallpapers', []);
export const currentWallpaper = logged('currentWallpaper', null);
export const view = logged('view', 'grid');
export const isLoading = logged('isLoading', false);
export const stats = logged('stats', {});
export const downloadQueue = logged('downloadQueue', { active: 0, pending: 0 });
export const previewOpen = logged('previewOpen', false);
export const favoritesOnly = logged('favoritesOnly', false);
export const currentPage = logged('currentPage', 1);
export const totalResults = logged('totalResults', 0);
export const pageSize = 50;
export const sortBy = logged('sortBy', 'random');

export const isFirstRun = logged('isFirstRun', true);
export const isScraping = logged('isScraping', false);
export const scrapeProgress = logged('scrapeProgress', { source: '', message: '' });
export const category = logged('category', '');
