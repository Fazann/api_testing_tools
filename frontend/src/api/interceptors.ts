// Interceptors run around every Go call made through request().
//
//   request  — runs before the call; throw to stop it.
//   response — may change the result after a successful call.
//   error    — sees every failure; throw to keep it failing, or return a value to recover.
//
// Interceptors run in the order they were added. use() returns a function that removes it.
// Calls to the remote backend server happen in Go (internal/backend), which has its own
// interceptors for auth, retries and the user agent.
import type { Endpoint } from './endpoints';

export interface RequestContext {
  endpoint: Endpoint;
}

export type RequestInterceptor = (ctx: RequestContext) => void | Promise<void>;
export type ResponseInterceptor = (data: unknown, ctx: RequestContext) => unknown;
export type ErrorInterceptor = (error: Error, ctx: RequestContext) => unknown;

class InterceptorList<T> {
  private items: T[] = [];

  use(fn: T): () => void {
    this.items.push(fn);
    return () => {
      this.items = this.items.filter((i) => i !== fn);
    };
  }

  clear() {
    this.items = [];
  }

  all(): readonly T[] {
    return [...this.items];
  }
}

export const interceptors = {
  request: new InterceptorList<RequestInterceptor>(),
  response: new InterceptorList<ResponseInterceptor>(),
  error: new InterceptorList<ErrorInterceptor>(),
};

/** Turns anything thrown (Wails rejects with plain strings) into an Error. */
export function toError(e: unknown): Error {
  return e instanceof Error ? e : new Error(String(e));
}
