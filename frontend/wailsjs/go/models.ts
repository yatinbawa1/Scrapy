export namespace config {
	
	export class Config {
	    firstRun: boolean;
	    dataDir: string;
	    cacheDir: string;
	    thumbnailDir: string;
	    downloadDir: string;
	    maxCacheSizeMB: number;
	    concurrentDl: number;
	    theme: string;
	    dbPath: string;
	    enabledSources: string[];
	    searchTerms: string[];
	    modelDir: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.firstRun = source["firstRun"];
	        this.dataDir = source["dataDir"];
	        this.cacheDir = source["cacheDir"];
	        this.thumbnailDir = source["thumbnailDir"];
	        this.downloadDir = source["downloadDir"];
	        this.maxCacheSizeMB = source["maxCacheSizeMB"];
	        this.concurrentDl = source["concurrentDl"];
	        this.theme = source["theme"];
	        this.dbPath = source["dbPath"];
	        this.enabledSources = source["enabledSources"];
	        this.searchTerms = source["searchTerms"];
	        this.modelDir = source["modelDir"];
	    }
	}

}

export namespace database {
	
	export class Wallpaper {
	    id: number;
	    url: string;
	    localPath: string;
	    thumbnailUrl: string;
	    thumbnailPath: string;
	    width: number;
	    height: number;
	    filesize: number;
	    source: string;
	    searchTerm: string;
	    hash: string;
	    title: string;
	    description: string;
	    tags: string[];
	    isFavorite: boolean;
	    status: string;
	    brightness: number;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Wallpaper(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.localPath = source["localPath"];
	        this.thumbnailUrl = source["thumbnailUrl"];
	        this.thumbnailPath = source["thumbnailPath"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.filesize = source["filesize"];
	        this.source = source["source"];
	        this.searchTerm = source["searchTerm"];
	        this.hash = source["hash"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.tags = source["tags"];
	        this.isFavorite = source["isFavorite"];
	        this.status = source["status"];
	        this.brightness = source["brightness"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SearchResult {
	    wallpapers: Wallpaper[];
	    total: number;
	    page: number;
	    pageSize: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.wallpapers = this.convertValues(source["wallpapers"], Wallpaper);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Source {
	    id: number;
	    name: string;
	    baseUrl: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Source(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.baseUrl = source["baseUrl"];
	        this.enabled = source["enabled"];
	    }
	}

}

export namespace main {
	
	export class AnalysisStatus {
	    submitted: number;
	    done: number;
	    active: number;
	    paused: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AnalysisStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.submitted = source["submitted"];
	        this.done = source["done"];
	        this.active = source["active"];
	        this.paused = source["paused"];
	    }
	}
	export class Collection {
	    name: string;
	    count: number;
	    sampleIds: number[];
	
	    static createFrom(source: any = {}) {
	        return new Collection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	        this.sampleIds = source["sampleIds"];
	    }
	}
	export class DuplicateGroup {
	    hash: string;
	    ids: number[];
	
	    static createFrom(source: any = {}) {
	        return new DuplicateGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.ids = source["ids"];
	    }
	}
	export class StorageItem {
	    key: string;
	    label: string;
	    path: string;
	    sizeBytes: number;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new StorageItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.path = source["path"];
	        this.sizeBytes = source["sizeBytes"];
	        this.count = source["count"];
	    }
	}

}

