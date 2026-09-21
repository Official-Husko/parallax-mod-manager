export namespace about {
	
	export class Link {
	    Icon: string;
	    Label: string;
	    URL: string;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Icon = source["Icon"];
	        this.Label = source["Label"];
	        this.URL = source["URL"];
	    }
	}
	export class Info {
	    Name: string;
	    Version: string;
	    Commit: string;
	    Dirty: boolean;
	    GoVersion: string;
	    WailsVersion: string;
	    OS: string;
	    Arch: string;
	    ConfigDir: string;
	    CacheDir: string;
	    LogDir: string;
	    Games: number;
	    Author: string;
	    LicenceName: string;
	    LicenceID: string;
	    Links: Link[];
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Version = source["Version"];
	        this.Commit = source["Commit"];
	        this.Dirty = source["Dirty"];
	        this.GoVersion = source["GoVersion"];
	        this.WailsVersion = source["WailsVersion"];
	        this.OS = source["OS"];
	        this.Arch = source["Arch"];
	        this.ConfigDir = source["ConfigDir"];
	        this.CacheDir = source["CacheDir"];
	        this.LogDir = source["LogDir"];
	        this.Games = source["Games"];
	        this.Author = source["Author"];
	        this.LicenceName = source["LicenceName"];
	        this.LicenceID = source["LicenceID"];
	        this.Links = this.convertValues(source["Links"], Link);
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

}

export namespace applog {
	
	export class Entry {
	    Seq: number;
	    Time: number;
	    Level: string;
	    Component: string;
	    Message: string;
	    Timed: boolean;
	    DurationMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Seq = source["Seq"];
	        this.Time = source["Time"];
	        this.Level = source["Level"];
	        this.Component = source["Component"];
	        this.Message = source["Message"];
	        this.Timed = source["Timed"];
	        this.DurationMs = source["DurationMs"];
	    }
	}

}

export namespace collection {
	
	export class ModRef {
	    gameId: string;
	    modId: string;
	
	    static createFrom(source: any = {}) {
	        return new ModRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameId = source["gameId"];
	        this.modId = source["modId"];
	    }
	}
	export class Collection {
	    name: string;
	    mods: ModRef[];
	
	    static createFrom(source: any = {}) {
	        return new Collection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.mods = this.convertValues(source["mods"], ModRef);
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

}

export namespace dlc {
	
	export class Entry {
	    ID: string;
	    Name: string;
	    Category: string;
	    SteamID: string;
	    SizeBytes: number;
	    Installed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Category = source["Category"];
	        this.SteamID = source["SteamID"];
	        this.SizeBytes = source["SizeBytes"];
	        this.Installed = source["Installed"];
	    }
	}

}

export namespace dlcstore {
	
	export class StoreData {
	    SteamAppID: string;
	    Name: string;
	    ShortDescription: string;
	    HeaderImage: string;
	    ReleaseDate: string;
	    ComingSoon: boolean;
	    Screenshots: string[];
	
	    static createFrom(source: any = {}) {
	        return new StoreData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SteamAppID = source["SteamAppID"];
	        this.Name = source["Name"];
	        this.ShortDescription = source["ShortDescription"];
	        this.HeaderImage = source["HeaderImage"];
	        this.ReleaseDate = source["ReleaseDate"];
	        this.ComingSoon = source["ComingSoon"];
	        this.Screenshots = source["Screenshots"];
	    }
	}

}

export namespace gamelog {
	
	export class File {
	    Name: string;
	    Size: number;
	    Modified: number;
	
	    static createFrom(source: any = {}) {
	        return new File(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Size = source["Size"];
	        this.Modified = source["Modified"];
	    }
	}
	export class Listing {
	    Dir: string;
	    Files: File[];
	
	    static createFrom(source: any = {}) {
	        return new Listing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Dir = source["Dir"];
	        this.Files = this.convertValues(source["Files"], File);
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

}

export namespace gameproc {
	
	export class Status {
	    Running: boolean;
	    PIDs: number[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Running = source["Running"];
	        this.PIDs = source["PIDs"];
	    }
	}

}

export namespace launcherdb {
	
	export class PlaysetMod {
	    GameRegistryID: string;
	    DisplayName: string;
	    Enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PlaysetMod(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.GameRegistryID = source["GameRegistryID"];
	        this.DisplayName = source["DisplayName"];
	        this.Enabled = source["Enabled"];
	    }
	}
	export class Playset {
	    ID: string;
	    Name: string;
	    IsActive: boolean;
	    Mods: PlaysetMod[];
	
	    static createFrom(source: any = {}) {
	        return new Playset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.IsActive = source["IsActive"];
	        this.Mods = this.convertValues(source["Mods"], PlaysetMod);
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

}

export namespace library {
	
	export class AuthorProfile {
	    SteamID: string;
	    Profile: steamapi.Profile;
	
	    static createFrom(source: any = {}) {
	        return new AuthorProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SteamID = source["SteamID"];
	        this.Profile = this.convertValues(source["Profile"], steamapi.Profile);
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
	export class ConflictCandidate {
	    ModID: string;
	    ModName: string;
	    FilePath: string;
	
	    static createFrom(source: any = {}) {
	        return new ConflictCandidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ModID = source["ModID"];
	        this.ModName = source["ModName"];
	        this.FilePath = source["FilePath"];
	    }
	}
	export class ConflictSummary {
	    Type: string;
	    ID: string;
	    Candidates: ConflictCandidate[];
	    Winner: string;
	    Overridden: boolean;
	    PatchState: string;
	    PatchNote: string;
	
	    static createFrom(source: any = {}) {
	        return new ConflictSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Type = source["Type"];
	        this.ID = source["ID"];
	        this.Candidates = this.convertValues(source["Candidates"], ConflictCandidate);
	        this.Winner = source["Winner"];
	        this.Overridden = source["Overridden"];
	        this.PatchState = source["PatchState"];
	        this.PatchNote = source["PatchNote"];
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
	export class DetectedGame {
	    ID: string;
	    DisplayName: string;
	    Installed: boolean;
	    InstallPath: string;
	    PathOverridden: boolean;
	    ModFolder: string;
	    ModCount: number;
	
	    static createFrom(source: any = {}) {
	        return new DetectedGame(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.DisplayName = source["DisplayName"];
	        this.Installed = source["Installed"];
	        this.InstallPath = source["InstallPath"];
	        this.PathOverridden = source["PathOverridden"];
	        this.ModFolder = source["ModFolder"];
	        this.ModCount = source["ModCount"];
	    }
	}
	export class EmptyModCandidate {
	    ID: string;
	    Name: string;
	    Reason: string;
	
	    static createFrom(source: any = {}) {
	        return new EmptyModCandidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Reason = source["Reason"];
	    }
	}
	export class FileEntry {
	    RelPath: string;
	    IsDir: boolean;
	    Size: number;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.RelPath = source["RelPath"];
	        this.IsDir = source["IsDir"];
	        this.Size = source["Size"];
	    }
	}
	export class GameInfo {
	    ID: string;
	    DisplayName: string;
	
	    static createFrom(source: any = {}) {
	        return new GameInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.DisplayName = source["DisplayName"];
	    }
	}
	export class GameUpdate {
	    GameID: string;
	    GameName: string;
	    From: string;
	    To: string;
	
	    static createFrom(source: any = {}) {
	        return new GameUpdate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.GameID = source["GameID"];
	        this.GameName = source["GameName"];
	        this.From = source["From"];
	        this.To = source["To"];
	    }
	}
	export class ModFileContent {
	    Content: string;
	    ModifiedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ModFileContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Content = source["Content"];
	        this.ModifiedAt = source["ModifiedAt"];
	    }
	}
	export class ModFiles {
	    Entries: FileEntry[];
	    TotalSize: number;
	    Truncated: boolean;
	    LastModified: number;
	
	    static createFrom(source: any = {}) {
	        return new ModFiles(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Entries = this.convertValues(source["Entries"], FileEntry);
	        this.TotalSize = source["TotalSize"];
	        this.Truncated = source["Truncated"];
	        this.LastModified = source["LastModified"];
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
	export class ModSummary {
	    ID: string;
	    Name: string;
	    Version: string;
	    SupportedVersion: string;
	    Source: string;
	    Tags: string[];
	    Dependencies: string[];
	    RemoteFileID: string;
	    ShortDescription: string;
	    Enabled: boolean;
	    GeneratedPatch: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Version = source["Version"];
	        this.SupportedVersion = source["SupportedVersion"];
	        this.Source = source["Source"];
	        this.Tags = source["Tags"];
	        this.Dependencies = source["Dependencies"];
	        this.RemoteFileID = source["RemoteFileID"];
	        this.ShortDescription = source["ShortDescription"];
	        this.Enabled = source["Enabled"];
	        this.GeneratedPatch = source["GeneratedPatch"];
	    }
	}
	export class PatchResult {
	    Written: boolean;
	    PatchedKeys: number;
	    SkippedKeys: number;
	    ModID: string;
	    Generation: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Written = source["Written"];
	        this.PatchedKeys = source["PatchedKeys"];
	        this.SkippedKeys = source["SkippedKeys"];
	        this.ModID = source["ModID"];
	        this.Generation = source["Generation"];
	    }
	}
	export class PatchSummary {
	    Exists: boolean;
	    GeneratedAt: number;
	    Generation: number;
	    Patched: number;
	    Changed: number;
	    New: number;
	    Obsolete: number;
	    GeneratedForVersion: string;
	    GameVersion: string;
	    GameChanged: boolean;
	    ChangedMods: string[];
	
	    static createFrom(source: any = {}) {
	        return new PatchSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Exists = source["Exists"];
	        this.GeneratedAt = source["GeneratedAt"];
	        this.Generation = source["Generation"];
	        this.Patched = source["Patched"];
	        this.Changed = source["Changed"];
	        this.New = source["New"];
	        this.Obsolete = source["Obsolete"];
	        this.GeneratedForVersion = source["GeneratedForVersion"];
	        this.GameVersion = source["GameVersion"];
	        this.GameChanged = source["GameChanged"];
	        this.ChangedMods = source["ChangedMods"];
	    }
	}
	export class PurgeResult {
	    Deleted: string[];
	    Errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new PurgeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Deleted = source["Deleted"];
	        this.Errors = source["Errors"];
	    }
	}
	export class Summary {
	    Game: GameInfo;
	    Mods: ModSummary[];
	    Conflicts: ConflictSummary[];
	    Patch: PatchSummary;
	    Errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Game = this.convertValues(source["Game"], GameInfo);
	        this.Mods = this.convertValues(source["Mods"], ModSummary);
	        this.Conflicts = this.convertValues(source["Conflicts"], ConflictSummary);
	        this.Patch = this.convertValues(source["Patch"], PatchSummary);
	        this.Errors = source["Errors"];
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

}

export namespace main {
	
	export class BackgroundPack {
	    GameID: string;
	    Files: number;
	    Bytes: number;
	    LocalFiles: number;
	    LocalBytes: number;
	    MissingFiles: number;
	    MissingBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new BackgroundPack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.GameID = source["GameID"];
	        this.Files = source["Files"];
	        this.Bytes = source["Bytes"];
	        this.LocalFiles = source["LocalFiles"];
	        this.LocalBytes = source["LocalBytes"];
	        this.MissingFiles = source["MissingFiles"];
	        this.MissingBytes = source["MissingBytes"];
	    }
	}
	export class BackgroundCatalog {
	    Packs: BackgroundPack[];
	    RemoteError: string;
	    Folder: string;
	    Source: string;
	
	    static createFrom(source: any = {}) {
	        return new BackgroundCatalog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Packs = this.convertValues(source["Packs"], BackgroundPack);
	        this.RemoteError = source["RemoteError"];
	        this.Folder = source["Folder"];
	        this.Source = source["Source"];
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
	
	export class SteamAPIStatus {
	    Mode: string;
	    HasKey: boolean;
	    Fingerprint: string;
	    State: string;
	    ExhaustedUntil: number;
	    LastError: string;
	    ItemsFromKey: number;
	    ItemsFromFree: number;
	    Rescued: number;
	    Protection: string;
	
	    static createFrom(source: any = {}) {
	        return new SteamAPIStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Mode = source["Mode"];
	        this.HasKey = source["HasKey"];
	        this.Fingerprint = source["Fingerprint"];
	        this.State = source["State"];
	        this.ExhaustedUntil = source["ExhaustedUntil"];
	        this.LastError = source["LastError"];
	        this.ItemsFromKey = source["ItemsFromKey"];
	        this.ItemsFromFree = source["ItemsFromFree"];
	        this.Rescued = source["Rescued"];
	        this.Protection = source["Protection"];
	    }
	}

}

export namespace modupdates {
	
	export class Change {
	    ModID: string;
	    Name: string;
	    Source: string;
	    RemoteFileID: string;
	    Kind: string;
	    New: boolean;
	    FromVersion: string;
	    ToVersion: string;
	    WorkshopUpdated: number;
	    FilesChanged: boolean;
	    FilesDelta: number;
	    SizeDelta: number;
	    GoneSince: number;
	
	    static createFrom(source: any = {}) {
	        return new Change(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ModID = source["ModID"];
	        this.Name = source["Name"];
	        this.Source = source["Source"];
	        this.RemoteFileID = source["RemoteFileID"];
	        this.Kind = source["Kind"];
	        this.New = source["New"];
	        this.FromVersion = source["FromVersion"];
	        this.ToVersion = source["ToVersion"];
	        this.WorkshopUpdated = source["WorkshopUpdated"];
	        this.FilesChanged = source["FilesChanged"];
	        this.FilesDelta = source["FilesDelta"];
	        this.SizeDelta = source["SizeDelta"];
	        this.GoneSince = source["GoneSince"];
	    }
	}
	export class Report {
	    GameID: string;
	    CheckedAt: number;
	    BaselineAt: number;
	    Changes: Change[];
	    ModsChecked: number;
	    WorkshopChecked: boolean;
	    WorkshopError: string;
	    Unreadable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.GameID = source["GameID"];
	        this.CheckedAt = source["CheckedAt"];
	        this.BaselineAt = source["BaselineAt"];
	        this.Changes = this.convertValues(source["Changes"], Change);
	        this.ModsChecked = source["ModsChecked"];
	        this.WorkshopChecked = source["WorkshopChecked"];
	        this.WorkshopError = source["WorkshopError"];
	        this.Unreadable = source["Unreadable"];
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

}

export namespace playset {
	
	export class Playset {
	    name: string;
	    gameKey: string;
	    modIds: string[];
	    disabledDlc: string[];
	
	    static createFrom(source: any = {}) {
	        return new Playset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.gameKey = source["gameKey"];
	        this.modIds = source["modIds"];
	        this.disabledDlc = source["disabledDlc"];
	    }
	}

}

export namespace preferences {
	
	export class Preferences {
	    scanForNewMods: boolean;
	    closeAfterLaunch: boolean;
	    warnOnPatchMismatch: boolean;
	    lastSelectedGame: string;
	    autosortDependencies: boolean;
	    autosortFixesLast: boolean;
	    autosortPatchLast: boolean;
	    managedGames: string[];
	    gamePaths: Record<string, string>;
	    extraModFolders: Record<string, Array<string>>;
	    backgroundDisabled: boolean;
	    backgroundRotationPaused: boolean;
	    backgroundIntervalSeconds: number;
	    backgroundSource: string;
	    launchModes: Record<string, string>;
	    lastActivePlaysets: Record<string, string>;
	    playsetAutoloadModes: Record<string, string>;
	    playsetAutoloadCustom: Record<string, string>;
	    lastSeenGameVersions: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Preferences(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanForNewMods = source["scanForNewMods"];
	        this.closeAfterLaunch = source["closeAfterLaunch"];
	        this.warnOnPatchMismatch = source["warnOnPatchMismatch"];
	        this.lastSelectedGame = source["lastSelectedGame"];
	        this.autosortDependencies = source["autosortDependencies"];
	        this.autosortFixesLast = source["autosortFixesLast"];
	        this.autosortPatchLast = source["autosortPatchLast"];
	        this.managedGames = source["managedGames"];
	        this.gamePaths = source["gamePaths"];
	        this.extraModFolders = source["extraModFolders"];
	        this.backgroundDisabled = source["backgroundDisabled"];
	        this.backgroundRotationPaused = source["backgroundRotationPaused"];
	        this.backgroundIntervalSeconds = source["backgroundIntervalSeconds"];
	        this.backgroundSource = source["backgroundSource"];
	        this.launchModes = source["launchModes"];
	        this.lastActivePlaysets = source["lastActivePlaysets"];
	        this.playsetAutoloadModes = source["playsetAutoloadModes"];
	        this.playsetAutoloadCustom = source["playsetAutoloadCustom"];
	        this.lastSeenGameVersions = source["lastSeenGameVersions"];
	    }
	}

}

export namespace steamapi {
	
	export class ChangelogEntry {
	    Headline: string;
	    Author: string;
	    AuthorProfileURL: string;
	    Body: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangelogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Headline = source["Headline"];
	        this.Author = source["Author"];
	        this.AuthorProfileURL = source["AuthorProfileURL"];
	        this.Body = source["Body"];
	    }
	}
	export class Profile {
	    Name: string;
	    AvatarURL: string;
	    ProfileURL: string;
	    MemberSince: string;
	    Location: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.AvatarURL = source["AvatarURL"];
	        this.ProfileURL = source["ProfileURL"];
	        this.MemberSince = source["MemberSince"];
	        this.Location = source["Location"];
	    }
	}
	export class PublishedFileDetails {
	    ID: string;
	    Result: number;
	    Banned: boolean;
	    Visibility: number;
	    Title: string;
	    Description: string;
	    PreviewURL: string;
	    Creator: string;
	    TimeCreated: number;
	    TimeUpdated: number;
	    Subscriptions: number;
	    Favorited: number;
	    Views: number;
	    FileSize: number;
	    Tags: string[];
	    Source: string;
	
	    static createFrom(source: any = {}) {
	        return new PublishedFileDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Result = source["Result"];
	        this.Banned = source["Banned"];
	        this.Visibility = source["Visibility"];
	        this.Title = source["Title"];
	        this.Description = source["Description"];
	        this.PreviewURL = source["PreviewURL"];
	        this.Creator = source["Creator"];
	        this.TimeCreated = source["TimeCreated"];
	        this.TimeUpdated = source["TimeUpdated"];
	        this.Subscriptions = source["Subscriptions"];
	        this.Favorited = source["Favorited"];
	        this.Views = source["Views"];
	        this.FileSize = source["FileSize"];
	        this.Tags = source["Tags"];
	        this.Source = source["Source"];
	    }
	}

}

