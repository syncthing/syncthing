import { Injectable } from '@angular/core';
import { environment } from 'src/environments/environment';
import { apiURL, apiRetry } from '../api-utils';
import { Observable } from 'rxjs';
import { HttpClient } from '@angular/common/http';
import { retry, map } from 'rxjs/operators';
import { SystemStatus } from '../system-status';

@Injectable({
  providedIn: 'root'
})
export class SystemStatusService {

  private systemStatusUrl = environment.production ? apiURL + 'rest/system/status' : 'api/systemStatus';

  constructor(private http: HttpClient) { }

  getSystemStatus(): Observable<SystemStatus> {
    return this.http
      .get<SystemStatus>(this.systemStatusUrl)
      .pipe(
        retry(apiRetry),
        map(res => {
          return res;
        })
      );
  }
}
