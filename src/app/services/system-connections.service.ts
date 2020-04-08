import { Injectable } from '@angular/core';
import { environment } from 'src/environments/environment';
import { apiURL } from '../api-utils';
import { HttpClient } from '@angular/common/http';
import { map } from 'rxjs/operators';
import { Observable } from 'rxjs';
import { SystemConnections } from '../connections';

@Injectable({
  providedIn: 'root'
})
export class SystemConnectionsService {
  private systemConfigUrl = environment.production ? apiURL + 'rest/system/connections' : 'api/connections';

  constructor(private http: HttpClient) { }

  getSystemConnections(): Observable<SystemConnections> {
    return this.http
      .get<SystemConnections>(this.systemConfigUrl)
      .pipe(
        map(res => {
          return res;
        })
      );
  }
}
