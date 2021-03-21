import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';

import { Observable, ReplaySubject } from 'rxjs';
import { map } from 'rxjs/operators';

import Folder from '../folder';
import Device from '../device';
import { environment } from '../../environments/environment'
import { apiURL } from '../api-utils'
import { ProgressService } from './progress.service';

@Injectable({
  providedIn: 'root'
})
export class SystemConfigService {
  private folders: Folder[];
  private devices: Device[];
  private foldersSubject: ReplaySubject<Folder[]> = new ReplaySubject(1);
  private devicesSubject: ReplaySubject<Device[]> = new ReplaySubject(1);

  private systemConfigUrl = environment.production ? apiURL + 'rest/system/config' : 'api/config';
  private guiConfigUrl = environment.production ? apiURL + 'rest/config/gui' : 'api/config/gui';

  constructor(
    private http: HttpClient,
    private progressService: ProgressService,
  ) { }

  getSystemConfig(): Observable<any> {
    return this.http
      .get(this.systemConfigUrl)
      .pipe(
        map(res => {
          this.folders = res['folders'];
          this.devices = res['devices'];

          // Set the total for the progress service
          this.progressService.total = this.folders.length + this.devices.length;

          this.foldersSubject.next(this.folders);
          this.devicesSubject.next(this.devices);

          return res;
        })
      );
  }

  getFolders(): Observable<Folder[]> {
    return this.foldersSubject.asObservable();
  }

  getDevices(): Observable<Device[]> {
    return this.devicesSubject.asObservable();
  }

  setGUITheme(theme: String): Observable<any> {
    return this.http.patch(this.guiConfigUrl, { theme: theme })
  }
}