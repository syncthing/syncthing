import { Injectable } from '@angular/core';
import { deviceID } from './api-utils';

@Injectable({
  providedIn: 'root'
})
export class CookieService {

  constructor() { }

  getCSRFHeader(): any {
    const dID: String = deviceID();
    const csrfCookie = 'CSRF-Token-' + dID
    const csrfHeader = {};
    csrfHeader['X-CSRF-Token-' + dID] = this.getCookie(csrfCookie);
    return csrfHeader;
  }

  getCookie(name: string): string {
    let ca: Array<string> = document.cookie.split(';');
    let caLen: number = ca.length;
    let cookieName = `${name}=`;
    let c: string;

    for (let i: number = 0; i < caLen; i += 1) {
      c = ca[i].replace(/^\s+/g, '');
      if (c.indexOf(cookieName) == 0) {
        return c.substring(cookieName.length, c.length);
      }
    }
    return '';
  }

  deleteCookie(name): void {
    this.setCookie(name, "", -1);
  }

  setCookie(name: string, value: string, expireDays: number, path: string = ""): void {
    let d: Date = new Date();
    d.setTime(d.getTime() + expireDays * 24 * 60 * 60 * 1000);
    let expires: string = "expires=" + d.toUTCString();
    document.cookie = name + "=" + value + "; " + expires + (path.length > 0 ? "; path=" + path : "");
  }
}
