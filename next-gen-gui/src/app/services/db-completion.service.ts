import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { environment } from 'src/environments/environment';
import { apiURL } from '../api-utils';
import { Completion } from '../completion';
import { map } from 'rxjs/operators';
import { Observable } from 'rxjs';
import { StType } from '../type';

@Injectable({
  providedIn: 'root'
})
export class DbCompletionService {
  private dbStatusUrl = environment.production ? apiURL + 'rest/db/completion' : 'api/dbCompletion';

  constructor(private http: HttpClient) { }

  getCompletion(type: StType, id: string): Observable<Completion> {
    let httpOptions: { params: HttpParams };
    if (id) {
      switch (type) {
        case StType.Device:
          httpOptions = {
            params: new HttpParams().set('device', id)
          };
          break;
        case StType.Folder:
          httpOptions = {
            params: new HttpParams().set('folder', id)
          };
          break;
      }
    } else { }

    return this.http
      .get<Completion>(this.dbStatusUrl, httpOptions)
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
