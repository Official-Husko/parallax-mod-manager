export namespace library {
	
	export class ConflictSummary {
	    Type: string;
	    ID: string;
	    Candidates: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConflictSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Type = source["Type"];
	        this.ID = source["ID"];
	        this.Candidates = source["Candidates"];
	    }
	}
	export class DetectedGame {
	    ID: string;
	    DisplayName: string;
	    Installed: boolean;
	    InstallPath: string;
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
	        this.ModFolder = source["ModFolder"];
	        this.ModCount = source["ModCount"];
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
	export class ModSummary {
	    ID: string;
	    Name: string;
	    Version: string;
	    Source: string;
	    Tags: string[];
	    Enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Version = source["Version"];
	        this.Source = source["Source"];
	        this.Tags = source["Tags"];
	        this.Enabled = source["Enabled"];
	    }
	}
	export class Summary {
	    Game: GameInfo;
	    Mods: ModSummary[];
	    Conflicts: ConflictSummary[];
	    Errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Game = this.convertValues(source["Game"], GameInfo);
	        this.Mods = this.convertValues(source["Mods"], ModSummary);
	        this.Conflicts = this.convertValues(source["Conflicts"], ConflictSummary);
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

