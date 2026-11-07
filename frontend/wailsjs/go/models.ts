export namespace backend {
	
	export class Status {
	    mode: string;
	    configured: boolean;
	    baseUrl: string;
	    reachable: boolean;
	    offline: boolean;
	    latencyMs: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.configured = source["configured"];
	        this.baseUrl = source["baseUrl"];
	        this.reachable = source["reachable"];
	        this.offline = source["offline"];
	        this.latencyMs = source["latencyMs"];
	        this.error = source["error"];
	    }
	}

}

export namespace collectionio {
	
	export class SecretFinding {
	    location: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.location = source["location"];
	        this.detail = source["detail"];
	    }
	}

}

export namespace compare {
	
	export class EnvRef {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new EnvRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class Side {
	    resultId: string;
	    status: string;
	    statusCode: number;
	    durationMs: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Side(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resultId = source["resultId"];
	        this.status = source["status"];
	        this.statusCode = source["statusCode"];
	        this.durationMs = source["durationMs"];
	        this.error = source["error"];
	    }
	}
	export class Result {
	    requestId: string;
	    name: string;
	    method: string;
	    verdict: string;
	    slower: string;
	    a: Side;
	    b: Side;
	    statusChanged: boolean;
	    body: domain.DiffEntry[];
	    bodyMessage?: string;
	    headers: domain.DiffEntry[];
	    changing: string[];
	    skipReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.name = source["name"];
	        this.method = source["method"];
	        this.verdict = source["verdict"];
	        this.slower = source["slower"];
	        this.a = this.convertValues(source["a"], Side);
	        this.b = this.convertValues(source["b"], Side);
	        this.statusChanged = source["statusChanged"];
	        this.body = this.convertValues(source["body"], domain.DiffEntry);
	        this.bodyMessage = source["bodyMessage"];
	        this.headers = this.convertValues(source["headers"], domain.DiffEntry);
	        this.changing = source["changing"];
	        this.skipReason = source["skipReason"];
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
	export class Report {
	    a: EnvRef;
	    b: EnvRef;
	    runIdA: string;
	    runIdB: string;
	    requests: Result[];
	    same: number;
	    different: number;
	    errors: number;
	    skipped: number;
	    slower: number;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.a = this.convertValues(source["a"], EnvRef);
	        this.b = this.convertValues(source["b"], EnvRef);
	        this.runIdA = source["runIdA"];
	        this.runIdB = source["runIdB"];
	        this.requests = this.convertValues(source["requests"], Result);
	        this.same = source["same"];
	        this.different = source["different"];
	        this.errors = source["errors"];
	        this.skipped = source["skipped"];
	        this.slower = source["slower"];
	        this.cancelled = source["cancelled"];
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

export namespace domain {
	
	export class Assertion {
	    id: string;
	    requestId: string;
	    kind: string;
	    path: string;
	    operator: string;
	    expected: number[];
	    tolerance: number;
	    function?: string;
	    description?: string;
	    enabled: boolean;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Assertion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.requestId = source["requestId"];
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.operator = source["operator"];
	        this.expected = source["expected"];
	        this.tolerance = source["tolerance"];
	        this.function = source["function"];
	        this.description = source["description"];
	        this.enabled = source["enabled"];
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
	export class AssertionResult {
	    assertionId: string;
	    kind: string;
	    path?: string;
	    operator?: string;
	    description?: string;
	    passed: boolean;
	    expected: string;
	    actual: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new AssertionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assertionId = source["assertionId"];
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.operator = source["operator"];
	        this.description = source["description"];
	        this.passed = source["passed"];
	        this.expected = source["expected"];
	        this.actual = source["actual"];
	        this.message = source["message"];
	    }
	}
	export class Auth {
	    type: string;
	    token?: string;
	    username?: string;
	    password?: string;
	    keyName?: string;
	    keyValue?: string;
	    keyIn?: string;
	
	    static createFrom(source: any = {}) {
	        return new Auth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.token = source["token"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.keyName = source["keyName"];
	        this.keyValue = source["keyValue"];
	        this.keyIn = source["keyIn"];
	    }
	}
	export class Baseline {
	    id: string;
	    requestId: string;
	    environmentId: string;
	    mode: string;
	    snapshot: number[];
	    selectedPaths: string[];
	    ignoredPaths: string[];
	    tolerance: number;
	    approvedBy: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Baseline(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.requestId = source["requestId"];
	        this.environmentId = source["environmentId"];
	        this.mode = source["mode"];
	        this.snapshot = source["snapshot"];
	        this.selectedPaths = source["selectedPaths"];
	        this.ignoredPaths = source["ignoredPaths"];
	        this.tolerance = source["tolerance"];
	        this.approvedBy = source["approvedBy"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class RuleTerm {
	    sign: string;
	    value: RuleValue;
	
	    static createFrom(source: any = {}) {
	        return new RuleTerm(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sign = source["sign"];
	        this.value = this.convertValues(source["value"], RuleValue);
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
	export class RuleValue {
	    source: string;
	    requestId?: string;
	    path?: string;
	    aggregate?: string;
	    literal?: string;
	
	    static createFrom(source: any = {}) {
	        return new RuleValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.requestId = source["requestId"];
	        this.path = source["path"];
	        this.aggregate = source["aggregate"];
	        this.literal = source["literal"];
	    }
	}
	export class Rule {
	    id: string;
	    name: string;
	    enabled: boolean;
	    kind: string;
	    left: RuleValue;
	    operator: string;
	    right: RuleTerm[];
	    tolerance: number;
	
	    static createFrom(source: any = {}) {
	        return new Rule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.kind = source["kind"];
	        this.left = this.convertValues(source["left"], RuleValue);
	        this.operator = source["operator"];
	        this.right = this.convertValues(source["right"], RuleTerm);
	        this.tolerance = source["tolerance"];
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
	export class Variable {
	    key: string;
	    value: string;
	    secret: boolean;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Variable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.secret = source["secret"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Collection {
	    id: string;
	    name: string;
	    description: string;
	    requiredVariables: string[];
	    variables: Variable[];
	    auth: Auth;
	    preRequestScript: string;
	    postResponseScript: string;
	    rules: Rule[];
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Collection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.requiredVariables = source["requiredVariables"];
	        this.variables = this.convertValues(source["variables"], Variable);
	        this.auth = this.convertValues(source["auth"], Auth);
	        this.preRequestScript = source["preRequestScript"];
	        this.postResponseScript = source["postResponseScript"];
	        this.rules = this.convertValues(source["rules"], Rule);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class SocketStep {
	    id: string;
	    kind: string;
	    description?: string;
	    enabled: boolean;
	    message: SocketMessage;
	    intervalMs?: number;
	    event?: string;
	    expectAck?: boolean;
	    assertions: Assertion[];
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new SocketStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.enabled = source["enabled"];
	        this.message = this.convertValues(source["message"], SocketMessage);
	        this.intervalMs = source["intervalMs"];
	        this.event = source["event"];
	        this.expectAck = source["expectAck"];
	        this.assertions = this.convertValues(source["assertions"], Assertion);
	        this.timeoutMs = source["timeoutMs"];
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
	export class SocketMessage {
	    id: string;
	    name: string;
	    event: string;
	    body: string;
	    bodyType: string;
	    ack?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SocketMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.event = source["event"];
	        this.body = source["body"];
	        this.bodyType = source["bodyType"];
	        this.ack = source["ack"];
	    }
	}
	export class SocketConfig {
	    subprotocols: string[];
	    path: string;
	    authPayload: string;
	    messages: SocketMessage[];
	    steps: SocketStep[];
	
	    static createFrom(source: any = {}) {
	        return new SocketConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subprotocols = source["subprotocols"];
	        this.path = source["path"];
	        this.authPayload = source["authPayload"];
	        this.messages = this.convertValues(source["messages"], SocketMessage);
	        this.steps = this.convertValues(source["steps"], SocketStep);
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
	export class FormField {
	    key: string;
	    value: string;
	    type: string;
	    file?: string;
	    contentType?: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FormField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.type = source["type"];
	        this.file = source["file"];
	        this.contentType = source["contentType"];
	        this.enabled = source["enabled"];
	    }
	}
	export class KeyValue {
	    key: string;
	    value: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KeyValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Request {
	    id: string;
	    collectionId: string;
	    parentId: string;
	    name: string;
	    kind: string;
	    method: string;
	    url: string;
	    headers: KeyValue[];
	    query: KeyValue[];
	    body: string;
	    bodyType: string;
	    form: FormField[];
	    bodyFile?: string;
	    auth: Auth;
	    timeoutMs: number;
	    followRedirects: boolean;
	    sortOrder: number;
	    assertions: Assertion[];
	    preRequestScript: string;
	    postResponseScript: string;
	    socket?: SocketConfig;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Request(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.collectionId = source["collectionId"];
	        this.parentId = source["parentId"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.method = source["method"];
	        this.url = source["url"];
	        this.headers = this.convertValues(source["headers"], KeyValue);
	        this.query = this.convertValues(source["query"], KeyValue);
	        this.body = source["body"];
	        this.bodyType = source["bodyType"];
	        this.form = this.convertValues(source["form"], FormField);
	        this.bodyFile = source["bodyFile"];
	        this.auth = this.convertValues(source["auth"], Auth);
	        this.timeoutMs = source["timeoutMs"];
	        this.followRedirects = source["followRedirects"];
	        this.sortOrder = source["sortOrder"];
	        this.assertions = this.convertValues(source["assertions"], Assertion);
	        this.preRequestScript = source["preRequestScript"];
	        this.postResponseScript = source["postResponseScript"];
	        this.socket = this.convertValues(source["socket"], SocketConfig);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class Folder {
	    id: string;
	    collectionId: string;
	    parentId: string;
	    name: string;
	    sortOrder: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.collectionId = source["collectionId"];
	        this.parentId = source["parentId"];
	        this.name = source["name"];
	        this.sortOrder = source["sortOrder"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class CollectionTree {
	    collection: Collection;
	    folders: Folder[];
	    requests: Request[];
	
	    static createFrom(source: any = {}) {
	        return new CollectionTree(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collection = this.convertValues(source["collection"], Collection);
	        this.folders = this.convertValues(source["folders"], Folder);
	        this.requests = this.convertValues(source["requests"], Request);
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
	export class DiffEntry {
	    path: string;
	    kind: string;
	    before?: string;
	    after?: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.before = source["before"];
	        this.after = source["after"];
	    }
	}
	export class EnvComparison {
	    collectionId: string;
	    envA: string;
	    envB: string;
	    ignoredPaths: string[];
	    includeModifying: boolean;
	    detectChanging: boolean;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new EnvComparison(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collectionId = source["collectionId"];
	        this.envA = source["envA"];
	        this.envB = source["envB"];
	        this.ignoredPaths = source["ignoredPaths"];
	        this.includeModifying = source["includeModifying"];
	        this.detectChanging = source["detectChanging"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class Environment {
	    id: string;
	    name: string;
	    variables: Variable[];
	    protected: boolean;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Environment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.variables = this.convertValues(source["variables"], Variable);
	        this.protected = source["protected"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	
	
	
	export class PerformanceCheck {
	    requestId: string;
	    mode: string;
	    factor: number;
	    minDeltaMs: number;
	    window: number;
	    minSamples: number;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new PerformanceCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.mode = source["mode"];
	        this.factor = source["factor"];
	        this.minDeltaMs = source["minDeltaMs"];
	        this.window = source["window"];
	        this.minSamples = source["minSamples"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class PerformanceResult {
	    mode: string;
	    referenceMs: number;
	    thresholdMs: number;
	    samples: number;
	    needed: number;
	    ratio: number;
	    slower: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new PerformanceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.referenceMs = source["referenceMs"];
	        this.thresholdMs = source["thresholdMs"];
	        this.samples = source["samples"];
	        this.needed = source["needed"];
	        this.ratio = source["ratio"];
	        this.slower = source["slower"];
	        this.message = source["message"];
	    }
	}
	export class Role {
	    id: string;
	    name: string;
	    anonymous: boolean;
	    variables: Variable[];
	
	    static createFrom(source: any = {}) {
	        return new Role(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.anonymous = source["anonymous"];
	        this.variables = this.convertValues(source["variables"], Variable);
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
	export class PermissionMatrix {
	    collectionId: string;
	    roles: Role[];
	    requestIds: string[];
	    expected: Record<string, any>;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new PermissionMatrix(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collectionId = source["collectionId"];
	        this.roles = this.convertValues(source["roles"], Role);
	        this.requestIds = source["requestIds"];
	        this.expected = source["expected"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class RaceCheck {
	    requestId: string;
	    path: string;
	    deltaPerSuccess: number;
	    tolerance: number;
	
	    static createFrom(source: any = {}) {
	        return new RaceCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.path = source["path"];
	        this.deltaPerSuccess = source["deltaPerSuccess"];
	        this.tolerance = source["tolerance"];
	    }
	}
	export class RaceTest {
	    requestId: string;
	    count: number;
	    successOp: string;
	    successCount: number;
	    uniquePath: string;
	    allowServerErrors: boolean;
	    check?: RaceCheck;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new RaceTest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.count = source["count"];
	        this.successOp = source["successOp"];
	        this.successCount = source["successCount"];
	        this.uniquePath = source["uniquePath"];
	        this.allowServerErrors = source["allowServerErrors"];
	        this.check = this.convertValues(source["check"], RaceCheck);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class RegressionResult {
	    baselineId: string;
	    passed: boolean;
	    diffs: DiffEntry[];
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new RegressionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baselineId = source["baselineId"];
	        this.passed = source["passed"];
	        this.diffs = this.convertValues(source["diffs"], DiffEntry);
	        this.message = source["message"];
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
	
	export class RequestSummary {
	    name: string;
	    method: string;
	    url: string;
	    headers: KeyValue[];
	    body?: string;
	
	    static createFrom(source: any = {}) {
	        return new RequestSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.method = source["method"];
	        this.url = source["url"];
	        this.headers = this.convertValues(source["headers"], KeyValue);
	        this.body = source["body"];
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
	export class ResponseSummary {
	    statusCode: number;
	    status: string;
	    headers: KeyValue[];
	    body: string;
	    bodyBytes: number;
	    truncated: boolean;
	    contentType: string;
	    redirects?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ResponseSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.statusCode = source["statusCode"];
	        this.status = source["status"];
	        this.headers = this.convertValues(source["headers"], KeyValue);
	        this.body = source["body"];
	        this.bodyBytes = source["bodyBytes"];
	        this.truncated = source["truncated"];
	        this.contentType = source["contentType"];
	        this.redirects = source["redirects"];
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
	export class ResponseTimePoint {
	    runId: string;
	    resultId: string;
	    // Go type: time
	    at: any;
	    durationMs: number;
	    status: string;
	    statusCode: number;
	    slower: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ResponseTimePoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.resultId = source["resultId"];
	        this.at = this.convertValues(source["at"], null);
	        this.durationMs = source["durationMs"];
	        this.status = source["status"];
	        this.statusCode = source["statusCode"];
	        this.slower = source["slower"];
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
	export class ResponseTimeStats {
	    count: number;
	    minMs: number;
	    medianMs: number;
	    p95Ms: number;
	    maxMs: number;
	
	    static createFrom(source: any = {}) {
	        return new ResponseTimeStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.minMs = source["minMs"];
	        this.medianMs = source["medianMs"];
	        this.p95Ms = source["p95Ms"];
	        this.maxMs = source["maxMs"];
	    }
	}
	export class ResponseTimeRow {
	    requestId: string;
	    name: string;
	    method: string;
	    parentId: string;
	    stats: ResponseTimeStats;
	    lastMs: number;
	    changePct?: number;
	    recent: number[];
	    lastSlower: boolean;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new ResponseTimeRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.name = source["name"];
	        this.method = source["method"];
	        this.parentId = source["parentId"];
	        this.stats = this.convertValues(source["stats"], ResponseTimeStats);
	        this.lastMs = source["lastMs"];
	        this.changePct = source["changePct"];
	        this.recent = source["recent"];
	        this.lastSlower = source["lastSlower"];
	        this.mode = source["mode"];
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
	
	export class ResponseTimeTrend {
	    requestId: string;
	    points: ResponseTimePoint[];
	    stats: ResponseTimeStats;
	    check: PerformanceCheck;
	    next: PerformanceResult;
	
	    static createFrom(source: any = {}) {
	        return new ResponseTimeTrend(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.points = this.convertValues(source["points"], ResponseTimePoint);
	        this.stats = this.convertValues(source["stats"], ResponseTimeStats);
	        this.check = this.convertValues(source["check"], PerformanceCheck);
	        this.next = this.convertValues(source["next"], PerformanceResult);
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
	
	
	
	
	export class SocketFrame {
	    direction: string;
	    event?: string;
	    data: string;
	    binary?: boolean;
	    ack?: boolean;
	    heartbeat?: boolean;
	    atMs: number;
	    truncated?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SocketFrame(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.direction = source["direction"];
	        this.event = source["event"];
	        this.data = source["data"];
	        this.binary = source["binary"];
	        this.ack = source["ack"];
	        this.heartbeat = source["heartbeat"];
	        this.atMs = source["atMs"];
	        this.truncated = source["truncated"];
	    }
	}
	export class SocketTranscript {
	    kind: string;
	    subprotocol?: string;
	    sessionId?: string;
	    frames: SocketFrame[];
	    dropped?: number;
	    closeCode?: number;
	    closeReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new SocketTranscript(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.subprotocol = source["subprotocol"];
	        this.sessionId = source["sessionId"];
	        this.frames = this.convertValues(source["frames"], SocketFrame);
	        this.dropped = source["dropped"];
	        this.closeCode = source["closeCode"];
	        this.closeReason = source["closeReason"];
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
	export class ScriptLog {
	    phase: string;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ScriptLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.level = source["level"];
	        this.message = source["message"];
	    }
	}
	export class RunResult {
	    id: string;
	    runId: string;
	    requestId: string;
	    request: RequestSummary;
	    response?: ResponseSummary;
	    assertionResults: AssertionResult[];
	    regression?: RegressionResult;
	    performance?: PerformanceResult;
	    logs?: ScriptLog[];
	    socket?: SocketTranscript;
	    durationMs: number;
	    status: string;
	    error?: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new RunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.runId = source["runId"];
	        this.requestId = source["requestId"];
	        this.request = this.convertValues(source["request"], RequestSummary);
	        this.response = this.convertValues(source["response"], ResponseSummary);
	        this.assertionResults = this.convertValues(source["assertionResults"], AssertionResult);
	        this.regression = this.convertValues(source["regression"], RegressionResult);
	        this.performance = this.convertValues(source["performance"], PerformanceResult);
	        this.logs = this.convertValues(source["logs"], ScriptLog);
	        this.socket = this.convertValues(source["socket"], SocketTranscript);
	        this.durationMs = source["durationMs"];
	        this.status = source["status"];
	        this.error = source["error"];
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
	export class Run {
	    id: string;
	    collectionId: string;
	    collectionName: string;
	    environmentName: string;
	    environmentId?: string;
	    kind?: string;
	    status: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	    passedCount: number;
	    failedCount: number;
	    errorCount: number;
	    durationMs: number;
	    results?: RunResult[];
	
	    static createFrom(source: any = {}) {
	        return new Run(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.collectionId = source["collectionId"];
	        this.collectionName = source["collectionName"];
	        this.environmentName = source["environmentName"];
	        this.environmentId = source["environmentId"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
	        this.passedCount = source["passedCount"];
	        this.failedCount = source["failedCount"];
	        this.errorCount = source["errorCount"];
	        this.durationMs = source["durationMs"];
	        this.results = this.convertValues(source["results"], RunResult);
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
	export class RunFilter {
	    collectionId: string;
	    requestId: string;
	    // Go type: time
	    from?: any;
	    // Go type: time
	    to?: any;
	    status: string;
	    limit: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new RunFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collectionId = source["collectionId"];
	        this.requestId = source["requestId"];
	        this.from = this.convertValues(source["from"], null);
	        this.to = this.convertValues(source["to"], null);
	        this.status = source["status"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
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
	
	
	export class Settings {
	    maxResponseBytes: number;
	    historyBodyLimitBytes: number;
	    historyRetentionRuns: number;
	    concurrency: number;
	    insecureSkipVerify: boolean;
	    caCertFile: string;
	    proxyMode: string;
	    proxyUrl: string;
	    allowSensitiveDestinations: boolean;
	    checkForUpdates: boolean;
	    reviewerName: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxResponseBytes = source["maxResponseBytes"];
	        this.historyBodyLimitBytes = source["historyBodyLimitBytes"];
	        this.historyRetentionRuns = source["historyRetentionRuns"];
	        this.concurrency = source["concurrency"];
	        this.insecureSkipVerify = source["insecureSkipVerify"];
	        this.caCertFile = source["caCertFile"];
	        this.proxyMode = source["proxyMode"];
	        this.proxyUrl = source["proxyUrl"];
	        this.allowSensitiveDestinations = source["allowSensitiveDestinations"];
	        this.checkForUpdates = source["checkForUpdates"];
	        this.reviewerName = source["reviewerName"];
	    }
	}
	
	
	
	
	

}

export namespace regression {
	
	export class ChangingField {
	    path: string;
	    reason: string;
	    examples: string[];
	    suggested: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChangingField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.reason = source["reason"];
	        this.examples = source["examples"];
	        this.suggested = source["suggested"];
	    }
	}

}

export namespace rules {
	
	export class Outcome {
	    ruleId: string;
	    skipped: boolean;
	    reason?: string;
	    result: domain.AssertionResult;
	
	    static createFrom(source: any = {}) {
	        return new Outcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ruleId = source["ruleId"];
	        this.skipped = source["skipped"];
	        this.reason = source["reason"];
	        this.result = this.convertValues(source["result"], domain.AssertionResult);
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

export namespace security {
	
	export class MatrixCell {
	    requestId: string;
	    roleId: string;
	    expected: string;
	    outcome: string;
	    statusCode: number;
	    durationMs: number;
	    verdict: string;
	    message?: string;
	    resultId: string;
	    suggested: string;
	
	    static createFrom(source: any = {}) {
	        return new MatrixCell(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.roleId = source["roleId"];
	        this.expected = source["expected"];
	        this.outcome = source["outcome"];
	        this.statusCode = source["statusCode"];
	        this.durationMs = source["durationMs"];
	        this.verdict = source["verdict"];
	        this.message = source["message"];
	        this.resultId = source["resultId"];
	        this.suggested = source["suggested"];
	    }
	}
	export class MatrixRequest {
	    id: string;
	    name: string;
	    method: string;
	
	    static createFrom(source: any = {}) {
	        return new MatrixRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.method = source["method"];
	    }
	}
	export class MatrixRole {
	    id: string;
	    name: string;
	    anonymous: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MatrixRole(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.anonymous = source["anonymous"];
	    }
	}
	export class MatrixReport {
	    runId: string;
	    passed: boolean;
	    cancelled: boolean;
	    roles: MatrixRole[];
	    requests: MatrixRequest[];
	    cells: MatrixCell[];
	    passes: number;
	    failures: number;
	    warnings: number;
	    unset: number;
	    errors: number;
	
	    static createFrom(source: any = {}) {
	        return new MatrixReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.passed = source["passed"];
	        this.cancelled = source["cancelled"];
	        this.roles = this.convertValues(source["roles"], MatrixRole);
	        this.requests = this.convertValues(source["requests"], MatrixRequest);
	        this.cells = this.convertValues(source["cells"], MatrixCell);
	        this.passes = source["passes"];
	        this.failures = source["failures"];
	        this.warnings = source["warnings"];
	        this.unset = source["unset"];
	        this.errors = source["errors"];
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
	
	
	export class StatusCount {
	    code: number;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new StatusCount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.count = source["count"];
	    }
	}
	export class RaceReport {
	    runId: string;
	    passed: boolean;
	    cancelled: boolean;
	    count: number;
	    successes: number;
	    serverErrors: number;
	    noResponse: number;
	    statusCounts: StatusCount[];
	    minMs: number;
	    maxMs: number;
	    avgMs: number;
	    before?: number;
	    after?: number;
	    checks: domain.AssertionResult[];
	    results: domain.RunResult[];
	
	    static createFrom(source: any = {}) {
	        return new RaceReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.passed = source["passed"];
	        this.cancelled = source["cancelled"];
	        this.count = source["count"];
	        this.successes = source["successes"];
	        this.serverErrors = source["serverErrors"];
	        this.noResponse = source["noResponse"];
	        this.statusCounts = this.convertValues(source["statusCounts"], StatusCount);
	        this.minMs = source["minMs"];
	        this.maxMs = source["maxMs"];
	        this.avgMs = source["avgMs"];
	        this.before = source["before"];
	        this.after = source["after"];
	        this.checks = this.convertValues(source["checks"], domain.AssertionResult);
	        this.results = this.convertValues(source["results"], domain.RunResult);
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

export namespace service {
	
	export class AppInfo {
	    name: string;
	    version: string;
	    dataPath: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.dataPath = source["dataPath"];
	    }
	}
	export class ApproveBaselineInput {
	    requestId: string;
	    environmentId: string;
	    body: string;
	    truncated: boolean;
	    selectedPaths: string[];
	    ignoredPaths: string[];
	    tolerance: number;
	    mode: string;
	    approvedBy: string;
	
	    static createFrom(source: any = {}) {
	        return new ApproveBaselineInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.environmentId = source["environmentId"];
	        this.body = source["body"];
	        this.truncated = source["truncated"];
	        this.selectedPaths = source["selectedPaths"];
	        this.ignoredPaths = source["ignoredPaths"];
	        this.tolerance = source["tolerance"];
	        this.mode = source["mode"];
	        this.approvedBy = source["approvedBy"];
	    }
	}
	export class ChangingReport {
	    fields: regression.ChangingField[];
	    samples: number;
	    statusCodes: number[];
	    statusChanged: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChangingReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fields = this.convertValues(source["fields"], regression.ChangingField);
	        this.samples = source["samples"];
	        this.statusCodes = source["statusCodes"];
	        this.statusChanged = source["statusChanged"];
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
	export class OperationRow {
	    operation: string;
	    summary: string;
	    requests: number;
	
	    static createFrom(source: any = {}) {
	        return new OperationRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operation = source["operation"];
	        this.summary = source["summary"];
	        this.requests = source["requests"];
	    }
	}
	export class CoverageRow {
	    requestId: string;
	    name: string;
	    method: string;
	    operation: string;
	    problem?: string;
	
	    static createFrom(source: any = {}) {
	        return new CoverageRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.name = source["name"];
	        this.method = source["method"];
	        this.operation = source["operation"];
	        this.problem = source["problem"];
	    }
	}
	export class ContractCoverage {
	    requests: CoverageRow[];
	    operations: OperationRow[];
	
	    static createFrom(source: any = {}) {
	        return new ContractCoverage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requests = this.convertValues(source["requests"], CoverageRow);
	        this.operations = this.convertValues(source["operations"], OperationRow);
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
	export class ContractInfo {
	    attached: boolean;
	    sourceName: string;
	    title: string;
	    version: string;
	    operations: number;
	    enabled: boolean;
	    allowUndocumented: boolean;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ContractInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.attached = source["attached"];
	        this.sourceName = source["sourceName"];
	        this.title = source["title"];
	        this.version = source["version"];
	        this.operations = source["operations"];
	        this.enabled = source["enabled"];
	        this.allowUndocumented = source["allowUndocumented"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	
	export class DetectChangingInput {
	    request: domain.Request;
	    environmentId: string;
	    ignoredPaths: string[];
	    confirmed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DetectChangingInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.request = this.convertValues(source["request"], domain.Request);
	        this.environmentId = source["environmentId"];
	        this.ignoredPaths = source["ignoredPaths"];
	        this.confirmed = source["confirmed"];
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
	export class HeartbeatStatus {
	    running: boolean;
	    intervalMs: number;
	    sent: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new HeartbeatStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.intervalMs = source["intervalMs"];
	        this.sent = source["sent"];
	        this.error = source["error"];
	    }
	}
	export class ImportResult {
	    collection: domain.Collection;
	    requests: number;
	    folders: number;
	    environments: number;
	    baselines: number;
	    baselinesSkipped: number;
	    contract: boolean;
	    raceTests: number;
	    permissionMatrix: boolean;
	    filesCleared: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collection = this.convertValues(source["collection"], domain.Collection);
	        this.requests = source["requests"];
	        this.folders = source["folders"];
	        this.environments = source["environments"];
	        this.baselines = source["baselines"];
	        this.baselinesSkipped = source["baselinesSkipped"];
	        this.contract = source["contract"];
	        this.raceTests = source["raceTests"];
	        this.permissionMatrix = source["permissionMatrix"];
	        this.filesCleared = source["filesCleared"];
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
	
	export class PreviousComparison {
	    found: boolean;
	    previousRunId: string;
	    // Go type: time
	    previousStartedAt: any;
	    statusBefore: string;
	    statusAfter: string;
	    codeBefore: number;
	    codeAfter: number;
	    newlyFailing: string[];
	    diffs: domain.DiffEntry[];
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new PreviousComparison(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.previousRunId = source["previousRunId"];
	        this.previousStartedAt = this.convertValues(source["previousStartedAt"], null);
	        this.statusBefore = source["statusBefore"];
	        this.statusAfter = source["statusAfter"];
	        this.codeBefore = source["codeBefore"];
	        this.codeAfter = source["codeAfter"];
	        this.newlyFailing = source["newlyFailing"];
	        this.diffs = this.convertValues(source["diffs"], domain.DiffEntry);
	        this.message = source["message"];
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
	export class RulesCheck {
	    runId: string;
	    // Go type: time
	    startedAt: any;
	    environmentName: string;
	    outcomes: rules.Outcome[];
	
	    static createFrom(source: any = {}) {
	        return new RulesCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.environmentName = source["environmentName"];
	        this.outcomes = this.convertValues(source["outcomes"], rules.Outcome);
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
	export class SignoffInput {
	    collectionId: string;
	    environmentId: string;
	    release: string;
	    preparedBy: string;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new SignoffInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collectionId = source["collectionId"];
	        this.environmentId = source["environmentId"];
	        this.release = source["release"];
	        this.preparedBy = source["preparedBy"];
	        this.notes = source["notes"];
	    }
	}
	export class SignoffResult {
	    html: string;
	    fileName: string;
	    ready: boolean;
	    reasons: string[];
	
	    static createFrom(source: any = {}) {
	        return new SignoffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.html = source["html"];
	        this.fileName = source["fileName"];
	        this.ready = source["ready"];
	        this.reasons = source["reasons"];
	    }
	}
	export class SocketSession {
	    sessionId: string;
	    kind: string;
	    url: string;
	    subprotocol?: string;
	    engineSessionId?: string;
	    statusCode: number;
	    headers: domain.KeyValue[];
	    logs?: domain.ScriptLog[];
	
	    static createFrom(source: any = {}) {
	        return new SocketSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.kind = source["kind"];
	        this.url = source["url"];
	        this.subprotocol = source["subprotocol"];
	        this.engineSessionId = source["engineSessionId"];
	        this.statusCode = source["statusCode"];
	        this.headers = this.convertValues(source["headers"], domain.KeyValue);
	        this.logs = this.convertValues(source["logs"], domain.ScriptLog);
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
	export class Warning {
	    level: string;
	    code: string;
	    requiresConfirmation: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Warning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.code = source["code"];
	        this.requiresConfirmation = source["requiresConfirmation"];
	        this.message = source["message"];
	    }
	}

}

export namespace storage {
	
	export class DataInfo {
	    path: string;
	    sizeBytes: number;
	    collections: number;
	    requests: number;
	    environments: number;
	    runs: number;
	    baselines: number;
	
	    static createFrom(source: any = {}) {
	        return new DataInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.sizeBytes = source["sizeBytes"];
	        this.collections = source["collections"];
	        this.requests = source["requests"];
	        this.environments = source["environments"];
	        this.runs = source["runs"];
	        this.baselines = source["baselines"];
	    }
	}

}

export namespace updater {
	
	export class Info {
	    enabled: boolean;
	    current: string;
	    latest: string;
	    available: boolean;
	    notes: string;
	    pageUrl: string;
	    method: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.available = source["available"];
	        this.notes = source["notes"];
	        this.pageUrl = source["pageUrl"];
	        this.method = source["method"];
	    }
	}

}

