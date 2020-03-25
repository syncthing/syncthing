import { Injectable } from '@angular/core';
import { deviceID } from '../api-utils';
import {
  HttpInterceptor, HttpHandler, HttpRequest, HttpHeaders
} from '@angular/common/http';

import { CookieService } from '../services/cookie.service';


@Injectable()
export class CSRFInterceptor implements HttpInterceptor {

  constructor(private cookieService: CookieService) { }

  intercept(req: HttpRequest<any>, next: HttpHandler) {
    const dID: String = deviceID();
    const csrfCookie = 'CSRF-Token-' + dID

    // Clone the request and replace the original headers with
    // cloned headers, updated with the CSRF information.
    const csrfReq = req.clone({
      headers: req.headers.set('X-CSRF-Token-' + dID,
        this.cookieService.getCookie(csrfCookie))
    });

    // send cloned request with header to the next handler.
    return next.handle(csrfReq);
  }
}