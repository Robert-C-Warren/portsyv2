export namespace backend {
	
	export class CommitMeta {
	    id: string;
	    message: string;
	    timestamp: number;
	    userId?: string;
	    parentId?: string;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new CommitMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.message = source["message"];
	        this.timestamp = source["timestamp"];
	        this.userId = source["userId"];
	        this.parentId = source["parentId"];
	        this.status = source["status"];
	    }
	}
	export class AbletonProject {
	    name: string;
	    path: string;
	    alsFile: string;
	    hasPortsy: boolean;
	    lastCommit?: CommitMeta;
	
	    static createFrom(source: any = {}) {
	        return new AbletonProject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.alsFile = source["alsFile"];
	        this.hasPortsy = source["hasPortsy"];
	        this.lastCommit = this.convertValues(source["lastCommit"], CommitMeta);
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
	
	export class RootStatsResult {
	    dirCount: number;
	    isDriveRoot: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RootStatsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dirCount = source["dirCount"];
	        this.isDriveRoot = source["isDriveRoot"];
	    }
	}

}

