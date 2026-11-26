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
	export class DJConfig {
	    enabled: boolean;
	    provider: string;
	    model: string;
	    baseUrl: string;
	    apiKey?: string;
	    revision: number;
	    dailySegments: number;

	    static createFrom(source: any = {}) {
	        return new DJConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.revision = source["revision"];
	        this.dailySegments = source["dailySegments"];
	    }
	}
	export class DJSearchStatus {
	    source: string;
	    query: string;
	    found: number;
	    state: string;

	    static createFrom(source: any = {}) {
	        return new DJSearchStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.query = source["query"];
	        this.found = source["found"];
	        this.state = source["state"];
	    }
	}
	export class DJDiagnostic {
	    configRevision: number;
	    stage: string;
	    error?: string;
	    found: number;
	    candidates: number;
	    selected: number;
	    searches: DJSearchStatus[];
	    updatedAt: string;

	    static createFrom(source: any = {}) {
	        return new DJDiagnostic(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configRevision = source["configRevision"];
	        this.stage = source["stage"];
	        this.error = source["error"];
	        this.found = source["found"];
	        this.candidates = source["candidates"];
	        this.selected = source["selected"];
	        this.searches = this.convertValues(source["searches"], DJSearchStatus);
	        this.updatedAt = source["updatedAt"];
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

	export class DJStatus {
	    diagnostic: DJDiagnostic;
	    tokensToday: number;
	    usedToday: number;
	    events: Record<string, number>;
	    enabled: boolean;
	    configured: boolean;
	    provider: string;
	    model: string;
	    baseUrl: string;
	    dailySegments: number;
	    state: string;
	    prepared: number;
	    fallbacks: number;
	    updatedAt?: string;
	    trackCount: number;

	    static createFrom(source: any = {}) {
	        return new DJStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.diagnostic = this.convertValues(source["diagnostic"], DJDiagnostic);
	        this.tokensToday = source["tokensToday"];
	        this.usedToday = source["usedToday"];
	        this.events = source["events"];
	        this.enabled = source["enabled"];
	        this.configured = source["configured"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.baseUrl = source["baseUrl"];
	        this.dailySegments = source["dailySegments"];
	        this.state = source["state"];
	        this.prepared = source["prepared"];
	        this.fallbacks = source["fallbacks"];
	        this.updatedAt = source["updatedAt"];
	        this.trackCount = source["trackCount"];
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
	export class FavoritesImportResult {
	    source: string;
	    found: number;
	    imported: number;
	    skipped: number;

	    static createFrom(source: any = {}) {
	        return new FavoritesImportResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.found = source["found"];
	        this.imported = source["imported"];
	        this.skipped = source["skipped"];
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
	export class PlaybackPreferences {
	    preferLocal: boolean;
	    preferExplicit: boolean;
	    officialOnly: boolean;
	    sourcePriority: Record<string, number>;

	    static createFrom(source: any = {}) {
	        return new PlaybackPreferences(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preferLocal = source["preferLocal"];
	        this.preferExplicit = source["preferExplicit"];
	        this.officialOnly = source["officialOnly"];
	        this.sourcePriority = source["sourcePriority"];
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
	    releaseDate?: string;
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
	        this.releaseDate = source["releaseDate"];
	        this.artworkUrl = source["artworkUrl"];
	        this.kind = source["kind"];
	        this.trackCount = source["trackCount"];
	        this.externalUrl = source["externalUrl"];
	    }
	}
	export class Track {
	    djSessionId?: string;
	    djPlanId?: string;
	    forcedSource?: boolean;
	    recommendationReason?: string;
	    fromCache?: boolean;
	    id: string;
	    service: string;
	    title: string;
	    artists: string[];
	    album?: string;
	    releaseDate?: string;
	    genre?: string;
	    durationMs?: number;
	    playCount?: number;
	    quality?: string;
	    artworkUrl?: string;
	    externalUrl?: string;
	    playableKind: string;

	    static createFrom(source: any = {}) {
	        return new Track(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.djSessionId = source["djSessionId"];
	        this.djPlanId = source["djPlanId"];
	        this.forcedSource = source["forcedSource"];
	        this.recommendationReason = source["recommendationReason"];
	        this.fromCache = source["fromCache"];
	        this.id = source["id"];
	        this.service = source["service"];
	        this.title = source["title"];
	        this.artists = source["artists"];
	        this.album = source["album"];
	        this.releaseDate = source["releaseDate"];
	        this.genre = source["genre"];
	        this.durationMs = source["durationMs"];
	        this.playCount = source["playCount"];
	        this.quality = source["quality"];
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
	export class PlaybackCandidate {
	    id: string;
	    canonicalTrackId: string;
	    track: Track;
	    identityConfidence: number;
	    version: string;
	    explicit: boolean;
	    official: boolean;
	    availability: boolean;
	    sourcePriority: number;
	    selectionReason?: string;

	    static createFrom(source: any = {}) {
	        return new PlaybackCandidate(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.canonicalTrackId = source["canonicalTrackId"];
	        this.track = this.convertValues(source["track"], Track);
	        this.identityConfidence = source["identityConfidence"];
	        this.version = source["version"];
	        this.explicit = source["explicit"];
	        this.official = source["official"];
	        this.availability = source["availability"];
	        this.sourcePriority = source["sourcePriority"];
	        this.selectionReason = source["selectionReason"];
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
	export class Playlist {
	    id: string;
	    service: string;
	    name: string;
	    tracksTotal: number;
	    artworkUrl?: string;
	    externalUrl?: string;

	    static createFrom(source: any = {}) {
	        return new Playlist(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.service = source["service"];
	        this.name = source["name"];
	        this.tracksTotal = source["tracksTotal"];
	        this.artworkUrl = source["artworkUrl"];
	        this.externalUrl = source["externalUrl"];
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
	    provider?: string;

	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.synced = source["synced"];
	        this.plain = source["plain"];
	        this.instrumental = source["instrumental"];
	        this.found = source["found"];
	        this.provider = source["provider"];
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
	export class Job {
	    id: string;
	    kind: string;
	    state: string;
	    priority: number;
	    payloadJson: string;
	    attempt: number;
	    maxAttempts: number;
	    progress: number;
	    message: string;
	    error?: string;
	    runAfterMs: number;
	    createdAtMs: number;
	    updatedAtMs: number;
	    startedAtMs?: number;
	    finishedAtMs?: number;

	    static createFrom(source: any = {}) {
	        return new Job(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.state = source["state"];
	        this.priority = source["priority"];
	        this.payloadJson = source["payloadJson"];
	        this.attempt = source["attempt"];
	        this.maxAttempts = source["maxAttempts"];
	        this.progress = source["progress"];
	        this.message = source["message"];
	        this.error = source["error"];
	        this.runAfterMs = source["runAfterMs"];
	        this.createdAtMs = source["createdAtMs"];
	        this.updatedAtMs = source["updatedAtMs"];
	        this.startedAtMs = source["startedAtMs"];
	        this.finishedAtMs = source["finishedAtMs"];
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

