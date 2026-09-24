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

export namespace app {
	
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
	
	export class BackupCleanupPlan {
	    Kind: string;
	    Entries: backup.Entry[];
	    Bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new BackupCleanupPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Kind = source["Kind"];
	        this.Entries = this.convertValues(source["Entries"], backup.Entry);
	        this.Bytes = source["Bytes"];
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
	export class BackupDeleteResult {
	    Deleted: number;
	    Bytes: number;
	    Skipped: number;
	
	    static createFrom(source: any = {}) {
	        return new BackupDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Deleted = source["Deleted"];
	        this.Bytes = source["Bytes"];
	        this.Skipped = source["Skipped"];
	    }
	}
	export class BackupOverview {
	    WorkshopMods: number;
	    WorkshopBytes: number;
	    AtRiskMods: number;
	    BackedUpMods: number;
	    BackedUpBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new BackupOverview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.WorkshopMods = source["WorkshopMods"];
	        this.WorkshopBytes = source["WorkshopBytes"];
	        this.AtRiskMods = source["AtRiskMods"];
	        this.BackedUpMods = source["BackedUpMods"];
	        this.BackedUpBytes = source["BackedUpBytes"];
	    }
	}
	export class BackupStatus {
	    Mode: string;
	    CustomPath: string;
	    Root: string;
	    DefaultRoot: string;
	    FreeBytes: number;
	    Running: boolean;
	    LimitEnabled: boolean;
	    LimitBytes: number;
	    KeepFreeEnabled: boolean;
	    KeepFreeBytes: number;
	    UsedBytes: number;
	    LimitState: string;
	
	    static createFrom(source: any = {}) {
	        return new BackupStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Mode = source["Mode"];
	        this.CustomPath = source["CustomPath"];
	        this.Root = source["Root"];
	        this.DefaultRoot = source["DefaultRoot"];
	        this.FreeBytes = source["FreeBytes"];
	        this.Running = source["Running"];
	        this.LimitEnabled = source["LimitEnabled"];
	        this.LimitBytes = source["LimitBytes"];
	        this.KeepFreeEnabled = source["KeepFreeEnabled"];
	        this.KeepFreeBytes = source["KeepFreeBytes"];
	        this.UsedBytes = source["UsedBytes"];
	        this.LimitState = source["LimitState"];
	    }
	}
	export class BuiltInPriorityRuleEntry {
	    Type: string;
	    Rule: string;
	
	    static createFrom(source: any = {}) {
	        return new BuiltInPriorityRuleEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Type = source["Type"];
	        this.Rule = source["Rule"];
	    }
	}
	export class CheckResult {
	    Findings: modcheck.Finding[];
	    BaseGameChecked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Findings = this.convertValues(source["Findings"], modcheck.Finding);
	        this.BaseGameChecked = source["BaseGameChecked"];
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
	export class DeepLStatus {
	    HasKey: boolean;
	    Fingerprint: string;
	    Tier: string;
	    Unreadable: boolean;
	    Protection: string;
	
	    static createFrom(source: any = {}) {
	        return new DeepLStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.HasKey = source["HasKey"];
	        this.Fingerprint = source["Fingerprint"];
	        this.Tier = source["Tier"];
	        this.Unreadable = source["Unreadable"];
	        this.Protection = source["Protection"];
	    }
	}
	export class DeveloperToolsStatus {
	    Available: boolean;
	    Enabled: boolean;
	    OS: string;
	
	    static createFrom(source: any = {}) {
	        return new DeveloperToolsStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Available = source["Available"];
	        this.Enabled = source["Enabled"];
	        this.OS = source["OS"];
	    }
	}
	export class ThumbnailPreview {
	    DataURI: string;
	    Width: number;
	    Height: number;
	    SourceWidth: number;
	    SourceHeight: number;
	    Bytes: number;
	    SourceBytes: number;
	    Resized: boolean;
	    Warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ThumbnailPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DataURI = source["DataURI"];
	        this.Width = source["Width"];
	        this.Height = source["Height"];
	        this.SourceWidth = source["SourceWidth"];
	        this.SourceHeight = source["SourceHeight"];
	        this.Bytes = source["Bytes"];
	        this.SourceBytes = source["SourceBytes"];
	        this.Resized = source["Resized"];
	        this.Warnings = source["Warnings"];
	    }
	}
	export class EditPreviewFile {
	    Path: string;
	    Kind: string;
	    Create: boolean;
	    Changed: boolean;
	    Before: string;
	    After: string;
	
	    static createFrom(source: any = {}) {
	        return new EditPreviewFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Kind = source["Kind"];
	        this.Create = source["Create"];
	        this.Changed = source["Changed"];
	        this.Before = source["Before"];
	        this.After = source["After"];
	    }
	}
	export class DuplicatePreview {
	    Files: EditPreviewFile[];
	    Thumbnail?: ThumbnailPreview;
	    Problems: string[];
	    Warnings: string[];
	    Nothing: boolean;
	    SourceFiles: number;
	    SourceBytes: number;
	    Big: boolean;
	    FreeAtTarget: number;
	
	    static createFrom(source: any = {}) {
	        return new DuplicatePreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Files = this.convertValues(source["Files"], EditPreviewFile);
	        this.Thumbnail = this.convertValues(source["Thumbnail"], ThumbnailPreview);
	        this.Problems = source["Problems"];
	        this.Warnings = source["Warnings"];
	        this.Nothing = source["Nothing"];
	        this.SourceFiles = source["SourceFiles"];
	        this.SourceBytes = source["SourceBytes"];
	        this.Big = source["Big"];
	        this.FreeAtTarget = source["FreeAtTarget"];
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
	export class DuplicateRequest {
	    Name: string;
	    Location: string;
	
	    static createFrom(source: any = {}) {
	        return new DuplicateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Location = source["Location"];
	    }
	}
	export class EditFile {
	    Path: string;
	    Kind: string;
	    Exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EditFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Kind = source["Kind"];
	        this.Exists = source["Exists"];
	    }
	}
	export class EditHistory {
	    Count: number;
	    LastSavedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new EditHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Count = source["Count"];
	        this.LastSavedAt = source["LastSavedAt"];
	    }
	}
	export class EditInfo {
	    ModID: string;
	    Name: string;
	    Editable: boolean;
	    Reason: string;
	    Overridable: boolean;
	    ContentPath: string;
	    Fields: modedit.Fields;
	    Picture: string;
	    Files: EditFile[];
	    CanCreateDescriptor: boolean;
	    HistoryCount: number;
	    LastSavedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new EditInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ModID = source["ModID"];
	        this.Name = source["Name"];
	        this.Editable = source["Editable"];
	        this.Reason = source["Reason"];
	        this.Overridable = source["Overridable"];
	        this.ContentPath = source["ContentPath"];
	        this.Fields = this.convertValues(source["Fields"], modedit.Fields);
	        this.Picture = source["Picture"];
	        this.Files = this.convertValues(source["Files"], EditFile);
	        this.CanCreateDescriptor = source["CanCreateDescriptor"];
	        this.HistoryCount = source["HistoryCount"];
	        this.LastSavedAt = source["LastSavedAt"];
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
	export class EditPreview {
	    Files: EditPreviewFile[];
	    Thumbnail?: ThumbnailPreview;
	    Problems: string[];
	    Warnings: string[];
	    Nothing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EditPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Files = this.convertValues(source["Files"], EditPreviewFile);
	        this.Thumbnail = this.convertValues(source["Thumbnail"], ThumbnailPreview);
	        this.Problems = source["Problems"];
	        this.Warnings = source["Warnings"];
	        this.Nothing = source["Nothing"];
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
	
	export class LauncherShimStatus {
	    Supported: boolean;
	    State: string;
	    Error: string;
	    HasRun: boolean;
	    LastRun: launchershim.Status;
	
	    static createFrom(source: any = {}) {
	        return new LauncherShimStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Supported = source["Supported"];
	        this.State = source["State"];
	        this.Error = source["Error"];
	        this.HasRun = source["HasRun"];
	        this.LastRun = this.convertValues(source["LastRun"], launchershim.Status);
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
	export class LoversLabAccountProfile {
	    Username: string;
	    ProfileURL: string;
	    AvatarURL: string;
	
	    static createFrom(source: any = {}) {
	        return new LoversLabAccountProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Username = source["Username"];
	        this.ProfileURL = source["ProfileURL"];
	        this.AvatarURL = source["AvatarURL"];
	    }
	}
	export class LoversLabCommentList {
	    Posts: loverslab.Post[];
	    TotalPages: number;
	    HasTopic: boolean;
	    TopicAuthor: string;
	
	    static createFrom(source: any = {}) {
	        return new LoversLabCommentList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Posts = this.convertValues(source["Posts"], loverslab.Post);
	        this.TotalPages = source["TotalPages"];
	        this.HasTopic = source["HasTopic"];
	        this.TopicAuthor = source["TopicAuthor"];
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
	export class LoversLabFileSummary {
	    ID: number;
	    Title: string;
	    URL: string;
	    Author: string;
	    AuthorURL: string;
	    Updated: string;
	    ThumbnailURL: string;
	    AuthorAvatarURL: string;
	    Views: number;
	    RealUpdated: string;
	
	    static createFrom(source: any = {}) {
	        return new LoversLabFileSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Title = source["Title"];
	        this.URL = source["URL"];
	        this.Author = source["Author"];
	        this.AuthorURL = source["AuthorURL"];
	        this.Updated = source["Updated"];
	        this.ThumbnailURL = source["ThumbnailURL"];
	        this.AuthorAvatarURL = source["AuthorAvatarURL"];
	        this.Views = source["Views"];
	        this.RealUpdated = source["RealUpdated"];
	    }
	}
	export class LoversLabFileList {
	    Files: LoversLabFileSummary[];
	    TotalPages: number;
	
	    static createFrom(source: any = {}) {
	        return new LoversLabFileList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Files = this.convertValues(source["Files"], LoversLabFileSummary);
	        this.TotalPages = source["TotalPages"];
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
	
	export class LoversLabInstalledMod {
	    FileID: number;
	    Title: string;
	    FileURL: string;
	    InstalledAt: number;
	    InstalledDateModified: string;
	    ContentMissing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LoversLabInstalledMod(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.FileID = source["FileID"];
	        this.Title = source["Title"];
	        this.FileURL = source["FileURL"];
	        this.InstalledAt = source["InstalledAt"];
	        this.InstalledDateModified = source["InstalledDateModified"];
	        this.ContentMissing = source["ContentMissing"];
	    }
	}
	export class LoversLabStatus {
	    SignedIn: boolean;
	    Username: string;
	    Unreadable: boolean;
	    Protection: string;
	
	    static createFrom(source: any = {}) {
	        return new LoversLabStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SignedIn = source["SignedIn"];
	        this.Username = source["Username"];
	        this.Unreadable = source["Unreadable"];
	        this.Protection = source["Protection"];
	    }
	}
	export class ModEdit {
	    Fields: modedit.Fields;
	    ThumbnailFrom: string;
	    CreateDescriptor: boolean;
	    Force: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModEdit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Fields = this.convertValues(source["Fields"], modedit.Fields);
	        this.ThumbnailFrom = source["ThumbnailFrom"];
	        this.CreateDescriptor = source["CreateDescriptor"];
	        this.Force = source["Force"];
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
	export class NewModLocation {
	    Path: string;
	    Label: string;
	    Default: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NewModLocation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Label = source["Label"];
	        this.Default = source["Default"];
	    }
	}
	export class NewModRequest {
	    Fields: modedit.Fields;
	    Location: string;
	
	    static createFrom(source: any = {}) {
	        return new NewModRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Fields = this.convertValues(source["Fields"], modedit.Fields);
	        this.Location = source["Location"];
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
	export class PlaysetChecksumResult {
	    Status: string;
	    Checksum: string;
	    Files: number;
	    Mods: number;
	    Reason: string;
	    Warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new PlaysetChecksumResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Status = source["Status"];
	        this.Checksum = source["Checksum"];
	        this.Files = source["Files"];
	        this.Mods = source["Mods"];
	        this.Reason = source["Reason"];
	        this.Warnings = source["Warnings"];
	    }
	}
	export class SaveResult {
	    Files: string[];
	    SavedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new SaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Files = source["Files"];
	        this.SavedAt = source["SavedAt"];
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
	
	export class TranslateEligibility {
	    AuthorModeOffered: boolean;
	    AuthorModeOverridable: boolean;
	    AuthorModeReason: string;
	    DeepLKeyReady: boolean;
	    HasEnglishContent: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TranslateEligibility(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.AuthorModeOffered = source["AuthorModeOffered"];
	        this.AuthorModeOverridable = source["AuthorModeOverridable"];
	        this.AuthorModeReason = source["AuthorModeReason"];
	        this.DeepLKeyReady = source["DeepLKeyReady"];
	        this.HasEnglishContent = source["HasEnglishContent"];
	    }
	}
	export class TranslateLanguage {
	    Code: string;
	    Name: string;
	    Confirmed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TranslateLanguage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Code = source["Code"];
	        this.Name = source["Name"];
	        this.Confirmed = source["Confirmed"];
	    }
	}
	export class TranslateRequest {
	    Service: string;
	    TargetCode: string;
	    Mode: string;
	    Force: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TranslateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Service = source["Service"];
	        this.TargetCode = source["TargetCode"];
	        this.Mode = source["Mode"];
	        this.Force = source["Force"];
	    }
	}
	export class TranslateResult {
	    Translated: number;
	    AlreadyCovered: number;
	    CompanionModID: string;
	
	    static createFrom(source: any = {}) {
	        return new TranslateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Translated = source["Translated"];
	        this.AlreadyCovered = source["AlreadyCovered"];
	        this.CompanionModID = source["CompanionModID"];
	    }
	}
	export class WorkshopAvailability {
	    RemoteFileID: string;
	    State: string;
	    Reason: string;
	    BackedUpAt: number;
	    BackupState: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkshopAvailability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.RemoteFileID = source["RemoteFileID"];
	        this.State = source["State"];
	        this.Reason = source["Reason"];
	        this.BackedUpAt = source["BackedUpAt"];
	        this.BackupState = source["BackupState"];
	    }
	}
	export class WorkshopPublishRequest {
	    ItemID: string;
	    Title: string;
	    Description: string;
	    ChangeNote: string;
	    Visibility: string;
	    ExcludePaths: string[];
	
	    static createFrom(source: any = {}) {
	        return new WorkshopPublishRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ItemID = source["ItemID"];
	        this.Title = source["Title"];
	        this.Description = source["Description"];
	        this.ChangeNote = source["ChangeNote"];
	        this.Visibility = source["Visibility"];
	        this.ExcludePaths = source["ExcludePaths"];
	    }
	}
	export class WorkshopPublishResult {
	    PublishedFileID: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkshopPublishResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.PublishedFileID = source["PublishedFileID"];
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

export namespace backup {
	
	export class Entry {
	    ModID: string;
	    RemoteFileID: string;
	    Name: string;
	    Version: string;
	    BackedUpAt: number;
	    Reason: string;
	    Files: number;
	    Size: number;
	    Newest: number;
	    Complete: boolean;
	    Missing: number;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ModID = source["ModID"];
	        this.RemoteFileID = source["RemoteFileID"];
	        this.Name = source["Name"];
	        this.Version = source["Version"];
	        this.BackedUpAt = source["BackedUpAt"];
	        this.Reason = source["Reason"];
	        this.Files = source["Files"];
	        this.Size = source["Size"];
	        this.Newest = source["Newest"];
	        this.Complete = source["Complete"];
	        this.Missing = source["Missing"];
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

export namespace launchershim {
	
	export class Status {
	    // Go type: time
	    time: any;
	    resolvedExe: string;
	    args?: string[];
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.resolvedExe = source["resolvedExe"];
	        this.args = source["args"];
	        this.success = source["success"];
	        this.error = source["error"];
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

export namespace loverslab {
	
	export class Category {
	    ID: number;
	    Name: string;
	    URL: string;
	    Files: number;
	    Depth: number;
	
	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.URL = source["URL"];
	        this.Files = source["Files"];
	        this.Depth = source["Depth"];
	    }
	}
	export class DescriptionRun {
	    Text: string;
	    Bold: boolean;
	    Italic: boolean;
	    Underline: boolean;
	    LinkURL: string;
	    EmoteURL: string;
	    EmoteAlt: string;
	
	    static createFrom(source: any = {}) {
	        return new DescriptionRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Text = source["Text"];
	        this.Bold = source["Bold"];
	        this.Italic = source["Italic"];
	        this.Underline = source["Underline"];
	        this.LinkURL = source["LinkURL"];
	        this.EmoteURL = source["EmoteURL"];
	        this.EmoteAlt = source["EmoteAlt"];
	    }
	}
	export class DescriptionBlock {
	    ImageURL: string;
	    Runs: DescriptionRun[];
	    Heading: number;
	    Divider: boolean;
	    Quote: boolean;
	    ListItem: boolean;
	    QuotedAuthor: string;
	    QuotedBlocks: DescriptionBlock[];
	    EmbedURL: string;
	
	    static createFrom(source: any = {}) {
	        return new DescriptionBlock(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ImageURL = source["ImageURL"];
	        this.Runs = this.convertValues(source["Runs"], DescriptionRun);
	        this.Heading = source["Heading"];
	        this.Divider = source["Divider"];
	        this.Quote = source["Quote"];
	        this.ListItem = source["ListItem"];
	        this.QuotedAuthor = source["QuotedAuthor"];
	        this.QuotedBlocks = this.convertValues(source["QuotedBlocks"], DescriptionBlock);
	        this.EmbedURL = source["EmbedURL"];
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
	export class ChangelogEntry {
	    Version: string;
	    Released: string;
	    Description: string;
	    DescriptionBlocks: DescriptionBlock[];
	
	    static createFrom(source: any = {}) {
	        return new ChangelogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Version = source["Version"];
	        this.Released = source["Released"];
	        this.Description = source["Description"];
	        this.DescriptionBlocks = this.convertValues(source["DescriptionBlocks"], DescriptionBlock);
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
	
	
	export class FileAuthor {
	    Name: string;
	    URL: string;
	    ImageURL: string;
	
	    static createFrom(source: any = {}) {
	        return new FileAuthor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.URL = source["URL"];
	        this.ImageURL = source["ImageURL"];
	    }
	}
	export class Screenshot {
	    URL: string;
	    ThumbnailURL: string;
	
	    static createFrom(source: any = {}) {
	        return new Screenshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.URL = source["URL"];
	        this.ThumbnailURL = source["ThumbnailURL"];
	    }
	}
	export class FileDetail {
	    Title: string;
	    Description: string;
	    DescriptionBlocks: DescriptionBlock[];
	    Version: string;
	    FileSize: string;
	    Author: FileAuthor;
	    Screenshots: Screenshot[];
	    Views: number;
	    Downloads: number;
	    DateModified: string;
	
	    static createFrom(source: any = {}) {
	        return new FileDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Title = source["Title"];
	        this.Description = source["Description"];
	        this.DescriptionBlocks = this.convertValues(source["DescriptionBlocks"], DescriptionBlock);
	        this.Version = source["Version"];
	        this.FileSize = source["FileSize"];
	        this.Author = this.convertValues(source["Author"], FileAuthor);
	        this.Screenshots = this.convertValues(source["Screenshots"], Screenshot);
	        this.Views = source["Views"];
	        this.Downloads = source["Downloads"];
	        this.DateModified = source["DateModified"];
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
	export class FileDownload {
	    Name: string;
	    URL: string;
	    Size: string;
	    Posted: string;
	
	    static createFrom(source: any = {}) {
	        return new FileDownload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.URL = source["URL"];
	        this.Size = source["Size"];
	        this.Posted = source["Posted"];
	    }
	}
	export class FileSummary {
	    ID: number;
	    Title: string;
	    URL: string;
	    Author: string;
	    AuthorURL: string;
	    Updated: string;
	    ThumbnailURL: string;
	
	    static createFrom(source: any = {}) {
	        return new FileSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Title = source["Title"];
	        this.URL = source["URL"];
	        this.Author = source["Author"];
	        this.AuthorURL = source["AuthorURL"];
	        this.Updated = source["Updated"];
	        this.ThumbnailURL = source["ThumbnailURL"];
	    }
	}
	export class PostAttachment {
	    Filename: string;
	    Extension: string;
	    URL: string;
	    IsImage: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PostAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Filename = source["Filename"];
	        this.Extension = source["Extension"];
	        this.URL = source["URL"];
	        this.IsImage = source["IsImage"];
	    }
	}
	export class Post {
	    ID: string;
	    Author: string;
	    AuthorURL: string;
	    AuthorAvatarURL: string;
	    AuthorGroup: string;
	    AuthorPostCount: number;
	    AuthorTitle: string;
	    IsTopicAuthor: boolean;
	    IsPopular: boolean;
	    Reactions: number;
	    Edited: boolean;
	    Posted: string;
	    URL: string;
	    Content: string;
	    ContentBlocks: DescriptionBlock[];
	    Attachments: PostAttachment[];
	
	    static createFrom(source: any = {}) {
	        return new Post(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Author = source["Author"];
	        this.AuthorURL = source["AuthorURL"];
	        this.AuthorAvatarURL = source["AuthorAvatarURL"];
	        this.AuthorGroup = source["AuthorGroup"];
	        this.AuthorPostCount = source["AuthorPostCount"];
	        this.AuthorTitle = source["AuthorTitle"];
	        this.IsTopicAuthor = source["IsTopicAuthor"];
	        this.IsPopular = source["IsPopular"];
	        this.Reactions = source["Reactions"];
	        this.Edited = source["Edited"];
	        this.Posted = source["Posted"];
	        this.URL = source["URL"];
	        this.Content = source["Content"];
	        this.ContentBlocks = this.convertValues(source["ContentBlocks"], DescriptionBlock);
	        this.Attachments = this.convertValues(source["Attachments"], PostAttachment);
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

export namespace modcheck {
	
	export class Finding {
	    Category: string;
	    Severity: string;
	    File: string;
	    Line: number;
	    Message: string;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Category = source["Category"];
	        this.Severity = source["Severity"];
	        this.File = source["File"];
	        this.Line = source["Line"];
	        this.Message = source["Message"];
	    }
	}

}

export namespace modedit {
	
	export class Fields {
	    Name: string;
	    Version: string;
	    SupportedVersion: string;
	    Tags: string[];
	    Dependencies: string[];
	    ReplacePaths: string[];
	
	    static createFrom(source: any = {}) {
	        return new Fields(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Version = source["Version"];
	        this.SupportedVersion = source["SupportedVersion"];
	        this.Tags = source["Tags"];
	        this.Dependencies = source["Dependencies"];
	        this.ReplacePaths = source["ReplacePaths"];
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
	    lockedModIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new Playset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.gameKey = source["gameKey"];
	        this.modIds = source["modIds"];
	        this.disabledDlc = source["disabledDlc"];
	        this.lockedModIds = source["lockedModIds"];
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
	    backgroundStaticImages: Record<string, string>;
	    backgroundBlur: number;
	    backgroundDarken: number;
	    launchModes: Record<string, string>;
	    lastActivePlaysets: Record<string, string>;
	    playsetAutoloadModes: Record<string, string>;
	    playsetAutoloadCustom: Record<string, string>;
	    lastSeenGameVersions: Record<string, string>;
	    accentMode: string;
	    accentColor: string;
	    developerTools: boolean;
	    loversLabCheckUpdates: boolean;
	    loversLabCheckIntervalHours: number;
	    loversLabNotifications: boolean;
	    loversLabNotificationIntervalMinutes: number;
	
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
	        this.backgroundStaticImages = source["backgroundStaticImages"];
	        this.backgroundBlur = source["backgroundBlur"];
	        this.backgroundDarken = source["backgroundDarken"];
	        this.launchModes = source["launchModes"];
	        this.lastActivePlaysets = source["lastActivePlaysets"];
	        this.playsetAutoloadModes = source["playsetAutoloadModes"];
	        this.playsetAutoloadCustom = source["playsetAutoloadCustom"];
	        this.lastSeenGameVersions = source["lastSeenGameVersions"];
	        this.accentMode = source["accentMode"];
	        this.accentColor = source["accentColor"];
	        this.developerTools = source["developerTools"];
	        this.loversLabCheckUpdates = source["loversLabCheckUpdates"];
	        this.loversLabCheckIntervalHours = source["loversLabCheckIntervalHours"];
	        this.loversLabNotifications = source["loversLabNotifications"];
	        this.loversLabNotificationIntervalMinutes = source["loversLabNotificationIntervalMinutes"];
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

