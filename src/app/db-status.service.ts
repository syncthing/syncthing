import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { CookieService } from './cookie.service';

import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

import { environment } from '../environments/environment'
import { apiURL } from './api-utils'

@Injectable({
  providedIn: 'root'
})
export class DbStatusService {
  private httpOptions: any;
  private dbStatusUrl = environment.production ? apiURL + 'rest/db/status' : 'api/dbStatus';

  constructor(private http: HttpClient, private cookieService: CookieService) {
    this.httpOptions = { headers: new HttpHeaders(this.cookieService.getCSRFHeader()) };
  }

  getFolderStatus(id: string): Observable<any> {
    return this.http
      .get(this.dbStatusUrl, this.httpOptions)
      .pipe(map(res => {
        // TODO update folder in system-config service
        return res;
      })
      );
  }
}
