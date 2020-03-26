import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { environment } from 'src/environments/environment';
import { apiURL, apiRetry } from '../api-utils';
import { Completion } from '../completion';
import { retry, map } from 'rxjs/operators';
import { Observable } from 'rxjs';

@Injectable({
  providedIn: 'root'
})
export class DbCompletionService {
  private dbStatusUrl = environment.production ? apiURL + 'rest/db/completion' : 'api/dbCompletion';

  constructor(private http: HttpClient) { }

  getDeviceCompletion(id: string): Observable<Completion> {
    let httpOptions: { params: HttpParams };
    if (id) {
      httpOptions = {
        params: new HttpParams().set('device', id)
      };
    } else { }

    return this.http
      .get<Completion>(this.dbStatusUrl, httpOptions)
      .pipe(
        retry(apiRetry),
        map(res => {
          // Remove from array in developement
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