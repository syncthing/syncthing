import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders, HttpParams } from '@angular/common/http';
import { CookieService } from './cookie.service';

import { Observable, of } from 'rxjs';
import { map, retry, catchError } from 'rxjs/operators';

import { environment } from '../environments/environment'
import { apiURL, apiRetry } from './api-utils'
import { FolderStatus, Folder } from './folder'

@Injectable({
  providedIn: 'root'
})
export class DbStatusService {
  private folderStatus: Object = {};

  // TODO why isn't this working?
  private httpOptions: { headers: HttpHeaders } | { params: HttpParams };
  private dbStatusUrl = environment.production ? apiURL + 'rest/db/status' : 'api/dbStatus';

  constructor(private http: HttpClient, private cookieService: CookieService) {
    this.httpOptions = { headers: new HttpHeaders(this.cookieService.getCSRFHeader()) };
  }

  getFolderStatus(id: string): Observable<FolderStatus> {
    /*
    if (id) {
      this.httpOptions["params"] = new HttpParams().set('folder', id);
    }
    */

    return this.http
      .get<FolderStatus>(this.dbStatusUrl, this.httpOptions)
      .pipe(
        retry(apiRetry),
        map(res => {
          return res;
        })
      );
  }
}