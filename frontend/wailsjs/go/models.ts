export namespace accounts {
	
	export class RuntimeState {
	    account_name: string;
	    cycle_status: string;
	    login_status: string;
	    last_error: string;
	    last_login_at: string;
	    last_cycle_started_at: string;
	    last_cycle_finished_at: string;
	    processed_challenges: number;
	    processed_sections: number;
	    remote_submission_count: number;
	    last_remote_submissions_sync_at: string;
	    session_token_file: string;
	    session_token_exists: boolean;
	    source: string;
	    updated_at: string;
	    raw_json?: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account_name = source["account_name"];
	        this.cycle_status = source["cycle_status"];
	        this.login_status = source["login_status"];
	        this.last_error = source["last_error"];
	        this.last_login_at = source["last_login_at"];
	        this.last_cycle_started_at = source["last_cycle_started_at"];
	        this.last_cycle_finished_at = source["last_cycle_finished_at"];
	        this.processed_challenges = source["processed_challenges"];
	        this.processed_sections = source["processed_sections"];
	        this.remote_submission_count = source["remote_submission_count"];
	        this.last_remote_submissions_sync_at = source["last_remote_submissions_sync_at"];
	        this.session_token_file = source["session_token_file"];
	        this.session_token_exists = source["session_token_exists"];
	        this.source = source["source"];
	        this.updated_at = source["updated_at"];
	        this.raw_json = source["raw_json"];
	    }
	}
	export class Account {
	    id: number;
	    name: string;
	    username: string;
	    password: string;
	    enabled: boolean;
	    submit_priority: number;
	    created_at: string;
	    updated_at: string;
	    runtime?: RuntimeState;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.enabled = source["enabled"];
	        this.submit_priority = source["submit_priority"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.runtime = this.convertValues(source["runtime"], RuntimeState);
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

export namespace combat {
	
	export class SubmitRequest {
	    account_name: string;
	    challenge_ids: string[];
	    flag: string;
	
	    static createFrom(source: any = {}) {
	        return new SubmitRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account_name = source["account_name"];
	        this.challenge_ids = source["challenge_ids"];
	        this.flag = source["flag"];
	    }
	}

}

export namespace logs {
	
	export class ContentRequest {
	    id: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new ContentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	    }
	}

}

export namespace pythonenv {
	
	export class InitRequest {
	    python_binary: string;
	
	    static createFrom(source: any = {}) {
	        return new InitRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.python_binary = source["python_binary"];
	    }
	}
	export class InstallPackagesRequest {
	    packages: string[];
	
	    static createFrom(source: any = {}) {
	        return new InstallPackagesRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.packages = source["packages"];
	    }
	}

}

export namespace sandbox {
	
	export class RunRequest {
	    code: string;
	    files: Record<string, string>;
	    args: string[];
	    python_binary: string;
	    timeout_seconds: number;
	    profile: string;
	
	    static createFrom(source: any = {}) {
	        return new RunRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.files = source["files"];
	        this.args = source["args"];
	        this.python_binary = source["python_binary"];
	        this.timeout_seconds = source["timeout_seconds"];
	        this.profile = source["profile"];
	    }
	}

}

export namespace tasks {
	
	export class NetworkProxySettings {
	    enabled: boolean;
	    type: string;
	    host: string;
	    port: number;
	    username: string;
	    password: string;
	    login_attempts: number;
	    login_retry_delay_seconds: number;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new NetworkProxySettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.type = source["type"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.login_attempts = source["login_attempts"];
	        this.login_retry_delay_seconds = source["login_retry_delay_seconds"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class StartRequest {
	    command: string;
	    account: string;
	    section: string;
	    ids: string;
	    flag: string;
	    workers: number;
	    force: boolean;
	    force_download: boolean;
	    force_solve: boolean;
	    no_submit: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	        this.account = source["account"];
	        this.section = source["section"];
	        this.ids = source["ids"];
	        this.flag = source["flag"];
	        this.workers = source["workers"];
	        this.force = source["force"];
	        this.force_download = source["force_download"];
	        this.force_solve = source["force_solve"];
	        this.no_submit = source["no_submit"];
	    }
	}

}

export namespace theory {
	
	export class AIReviewRequest {
	    limit: number;
	    batch_size: number;
	    timeout_seconds: number;
	    dry_run: boolean;
	    only_pending: boolean;
	    reasoning_effort: string;
	
	    static createFrom(source: any = {}) {
	        return new AIReviewRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.limit = source["limit"];
	        this.batch_size = source["batch_size"];
	        this.timeout_seconds = source["timeout_seconds"];
	        this.dry_run = source["dry_run"];
	        this.only_pending = source["only_pending"];
	        this.reasoning_effort = source["reasoning_effort"];
	    }
	}
	export class AISettings {
	    enabled: boolean;
	    base_url: string;
	    api_key: string;
	    model: string;
	    reasoning_effort: string;
	    prompt: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new AISettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.model = source["model"];
	        this.reasoning_effort = source["reasoning_effort"];
	        this.prompt = source["prompt"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class AutomationRequest {
	    max_questions: number;
	    allow_ai: boolean;
	    stop_on_no_answer: boolean;
	    account: string;
	
	    static createFrom(source: any = {}) {
	        return new AutomationRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_questions = source["max_questions"];
	        this.allow_ai = source["allow_ai"];
	        this.stop_on_no_answer = source["stop_on_no_answer"];
	        this.account = source["account"];
	    }
	}
	export class BankOption {
	    key: string;
	    content: string;
	    input_type?: string;
	    is_correct: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BankOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.content = source["content"];
	        this.input_type = source["input_type"];
	        this.is_correct = source["is_correct"];
	    }
	}
	export class ManualSubmitRequest {
	    account: string;
	    options: string[];
	
	    static createFrom(source: any = {}) {
	        return new ManualSubmitRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account = source["account"];
	        this.options = source["options"];
	    }
	}
	export class ReviewDecision {
	    id: number;
	    question: string;
	    selection_type: string;
	    options: BankOption[];
	    answer_keys: string[];
	    answer_texts: string[];
	    review_status: string;
	    review_reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewDecision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.question = source["question"];
	        this.selection_type = source["selection_type"];
	        this.options = this.convertValues(source["options"], BankOption);
	        this.answer_keys = source["answer_keys"];
	        this.answer_texts = source["answer_texts"];
	        this.review_status = source["review_status"];
	        this.review_reason = source["review_reason"];
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
	export class SnapshotRequest {
	    account: string;
	    refresh: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SnapshotRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account = source["account"];
	        this.refresh = source["refresh"];
	    }
	}

}

