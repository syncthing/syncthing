import { Injectable } from '@angular/core';
import {
  HttpRequest,
  HttpHandler,
  HttpEvent,
  HttpInterceptor,
  HttpHeaders,
  HttpResponse
} from '@angular/common/http';
import { Observable, of } from 'rxjs';
import { tap } from 'rxjs/operators';
import { RequestCacheService } from '../services/request-cache.service'

@Injectable()
export class CachingInterceptor implements HttpInterceptor {
  constructor(private cache: RequestCacheService) { }

  intercept(req: HttpRequest<any>, next: HttpHandler) {
    // continue if not cacheable.
    if (!isCacheable(req)) { return next.handle(req); }

    const cachedResponse = this.cache.get(req);
    return cachedResponse ?
      of(cachedResponse) : sendRequest(req, next, this.cache);
  }
}

/** Is this request cacheable? */
function isCacheable(req: HttpRequest<any>) {
  // Only GET requests are cacheable
  return req.method === 'GET';
  /*
  return req.method === 'GET' &&
    -1 < req.url.indexOf("url");
    */
}

/**
 * Get server response observable by sending request to `next()`.
 * Will add the response to the cache on the way out.
 */
function sendRequest(
  req: HttpRequest<any>,
  next: HttpHandler,
  cache: RequestCacheService): Observable<HttpEvent<any>> {

  // No headers allowed in npm search request
  const noHeaderReq = req.clone({ headers: new HttpHeaders() });

  return next.handle(noHeaderReq).pipe(
    tap(event => {
      // There may be other events besides the response.
      if (event instanceof HttpResponse) {
        // cache.put(req, event); // Update the cache.
      }
    })
  );
}
