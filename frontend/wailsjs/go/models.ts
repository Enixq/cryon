export namespace audiofetcher {
	
	export class AudioStream {
	    url: string;
	    format: string;
	    quality: string;
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new AudioStream(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.format = source["format"];
	        this.quality = source["quality"];
	        this.duration = source["duration"];
	    }
	}

}

export namespace core {
	
	export class AccountConnectionStatus {
	    service: string;
	    name: string;
	    oauthConfigured: boolean;
	    accountConnected: boolean;
	    canImportLibrary: boolean;
	    setupHint: string;
	
	    static createFrom(source: any = {}) {
	        return new AccountConnectionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.service = source["service"];
	        this.name = source["name"];
	        this.oauthConfigured = source["oauthConfigured"];
	        this.accountConnected = source["accountConnected"];
	        this.canImportLibrary = source["canImportLibrary"];
	        this.setupHint = source["setupHint"];
	    }
	}
	export class FavoritesImportResult {
	    source: string;
	    found: number;
	    imported: number;
	
	    static createFrom(source: any = {}) {
	        return new FavoritesImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.found = source["found"];
	        this.imported = source["imported"];
	    }
	}
	export class HistoryItem {
	    track: domain.Track;
	    playedAtMs: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.track = this.convertValues(source["track"], domain.Track);
	        this.playedAtMs = source["playedAtMs"];
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
	export class SourceStatus {
	    id: string;
	    connected: boolean;
	    level: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.connected = source["connected"];
	        this.level = source["level"];
	        this.error = source["error"];
	    }
	}

}

export namespace domain {
	
	export class Album {
	    id: string;
	    service: string;
	    title: string;
	    artist?: string;
	    year?: number;
	    artworkUrl?: string;
	    kind?: string;
	    trackCount?: number;
	    externalUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new Album(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.service = source["service"];
	        this.title = source["title"];
	        this.artist = source["artist"];
	        this.year = source["year"];
	        this.artworkUrl = source["artworkUrl"];
	        this.kind = source["kind"];
	        this.trackCount = source["trackCount"];
	        this.externalUrl = source["externalUrl"];
	    }
	}
	export class Track {
	    id: string;
	    service: string;
	    title: string;
	    artists: string[];
	    album?: string;
	    durationMs?: number;
    playCount?: number;
	    artworkUrl?: string;
	    externalUrl?: string;
	    playableKind: string;
	
	    static createFrom(source: any = {}) {
	        return new Track(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.service = source["service"];
	        this.title = source["title"];
	        this.artists = source["artists"];
	        this.album = source["album"];
	        this.durationMs = source["durationMs"];
        this.playCount = source["playCount"];
	        this.artworkUrl = source["artworkUrl"];
	        this.externalUrl = source["externalUrl"];
	        this.playableKind = source["playableKind"];
	    }
	}
	export class ArtistInfo {
	    name: string;
	    service?: string;
	    artworkUrl?: string;
	    topTracks: Track[];
	    albums: Album[];
	    singles: Album[];
	    appearsOn: Album[];
	
	    static createFrom(source: any = {}) {
	        return new ArtistInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.service = source["service"];
	        this.artworkUrl = source["artworkUrl"];
	        this.topTracks = this.convertValues(source["topTracks"], Track);
	        this.albums = this.convertValues(source["albums"], Album);
	        this.singles = this.convertValues(source["singles"], Album);
	        this.appearsOn = this.convertValues(source["appearsOn"], Album);
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
	
	export class UserPlaylist {
	    id: string;
	    title: string;
	    description: string;
	    trackCount: number;
	    createdAtMs?: number;
	    coverUrls?: string[];
	
	    static createFrom(source: any = {}) {
	        return new UserPlaylist(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.trackCount = source["trackCount"];
	        this.createdAtMs = source["createdAtMs"];
	        this.coverUrls = source["coverUrls"];
	    }
	}

}

export namespace lyrics {
	
	export class Result {
	    synced: string;
	    plain: string;
	    instrumental: boolean;
	    found: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.synced = source["synced"];
	        this.plain = source["plain"];
	        this.instrumental = source["instrumental"];
	        this.found = source["found"];
	    }
	}

}

export namespace playback {
	
	export class Status {
	    backend: string;
	    available: boolean;
	    state: string;
	    trackId?: string;
	    positionS: number;
	    durationS: number;
	    volume: number;
	    eof: boolean;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.available = source["available"];
	        this.state = source["state"];
	        this.trackId = source["trackId"];
	        this.positionS = source["positionS"];
	        this.durationS = source["durationS"];
	        this.volume = source["volume"];
	        this.eof = source["eof"];
	        this.message = source["message"];
	    }
	}

}

export namespace store {
	
	export class Account {
	    id: string;
	    login: string;
	    email: string;
	    isGuest: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.login = source["login"];
	        this.email = source["email"];
	        this.isGuest = source["isGuest"];
	    }
	}
	export class NotificationRow {
	    id: string;
	    kind: string;
	    title: string;
	    message: string;
	    createdAtMs: number;
	    read: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NotificationRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.createdAtMs = source["createdAtMs"];
	        this.read = source["read"];
	    }
	}
	export class PlaylistTrackKey {
	    service: string;
	    trackId: string;
	
	    static createFrom(source: any = {}) {
	        return new PlaylistTrackKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.service = source["service"];
	        this.trackId = source["trackId"];
	    }
	}

}

