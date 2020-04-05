import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';

import { Observable, Subject } from 'rxjs';
import { map, retry } from 'rxjs/operators';

import Folder from '../folder';
import Device from '../device';
import { environment } from '../../environments/environment'
import { apiURL, apiRetry } from '../api-utils'
import { ProgressService } from './progress.service';

@Injectable({
  providedIn: 'root'
})
export class SystemConfigService {
  private folders: Folder[];
  private devices: Device[];
  private foldersSubject: Subject<Folder[]> = new Subject();
  private devicesSubject: Subject<Device[]> = new Subject();

  private systemConfigUrl = environment.production ? apiURL + 'rest/system/config' : 'api/config';

  private checkInterval: number = 100;

  constructor(
    private http: HttpClient,
    private progressService: ProgressService,
  ) { }

  getSystemConfig(): Observable<any> {
    return this.http
      .get(this.systemConfigUrl)
      .pipe(
        retry(apiRetry),
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
    const folderObservable: Observable<Folder[]> = new Observable((observer) => {
      if (this.folders) {
        observer.next(this.folders);
        observer.complete();
      } else {
        // create timer to keep checking for folders
        let t = setInterval(() => {
          if (this.folders) {
            clearInterval(t);
            observer.next(this.folders)
            observer.complete();
          }
        }, this.checkInterval);
      }
    });
    return folderObservable;
  }

  getDevices(): Observable<Device[]> {
    const deviceObserverable: Observable<Device[]> = new Observable((observer) => {
      if (this.devices) {
        observer.next(this.devices);
      } else {
        let t = setInterval(() => {
          if (this.devices) {
            clearInterval(t);
            observer.next(this.devices);
          }
        }, this.checkInterval);
      }
    });
    return deviceObserverable;
  }
}