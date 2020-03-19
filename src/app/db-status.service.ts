import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders, HttpParams } from '@angular/common/http';
import { CookieService } from './cookie.service';

import { Observable, of } from 'rxjs';
import { map, retry, catchError } from 'rxjs/operators';

import { environment } from '../environments/environment'
import { apiURL, apiRetry } from './api-utils'
import Folder from './folder'

@Injectable({
  providedIn: 'root'
})
export class DbStatusService {
  private folderStatus: Object = {};
  private headers: HttpHeaders;
  private dbStatusUrl = environment.production ? apiURL + 'rest/db/status' : 'api/dbStatus';

  constructor(private http: HttpClient, private cookieService: CookieService) {
    this.headers = new HttpHeaders(this.cookieService.getCSRFHeader())
  }

  getFolderStatus(id: string): Observable<Folder.Status> {
    let httpOptions: { headers: HttpHeaders } |
    { headers: HttpHeaders, params: HttpParams };
    if (id) {
      httpOptions = {
        headers: this.headers,
        params: new HttpParams().set('folder', id)
      };
    } else {
      httpOptions = { headers: this.headers };
    }

    return this.http
      .get<Folder.Status>(this.dbStatusUrl, httpOptions)
      .pipe(
        retry(apiRetry),
        map(res => {
          // Remove from array in developement
          // in-memory-web-api returns arrays
          if (!environment.production) {
            console.log("status res!", res);
            const a: any = res as any;
            if (a.length > 0) {
              return res[0];
            }
            return {};
          }
          return res;
        })
      );
  }
}