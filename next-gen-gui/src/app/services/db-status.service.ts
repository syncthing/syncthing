import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders, HttpParams } from '@angular/common/http';

import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

import { environment } from '../../environments/environment'
import { apiURL } from '../api-utils'
import Folder from '../folder'

@Injectable({
  providedIn: 'root'
})
export class DbStatusService {
  private dbStatusUrl = environment.production ? apiURL + 'rest/db/status' : 'api/dbStatus';

  constructor(private http: HttpClient) { }

  getFolderStatus(id: string): Observable<Folder.Status> {
    let httpOptions: { params: HttpParams };
    if (id) {
      httpOptions = {
        params: new HttpParams().set('folder', id)
      };
    } else { }

    return this.http
      .get<Folder.Status>(this.dbStatusUrl, httpOptions)
      .pipe(
        map(res => {
          // Remove from array in development
          // in-memory-web-api returns arrays
          if (!environment.production) {
            const a: any = res as any;
            if (a.length > 0) {
              res = res[0];
            }
          }
          return res;
        })
      );
  }
}
