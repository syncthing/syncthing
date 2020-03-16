import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';

import { Observable, Subject, of } from 'rxjs';
import { map } from 'rxjs/operators';

import { Folder } from './folder';
import { Device } from './device';
import { FOLDERS, DEVICES } from './mock-config-data';
import { CookieService } from './cookie.service';
import { environment } from '../environments/environment'
import { apiURL } from './api-utils'

@Injectable({
  providedIn: 'root'
})
export class SystemConfigService {
  private systemConfig: any;
  private folders: Folder[];
  private devices: Device[];
  private foldersSubject: Subject<Folder[]> = new Subject();
  private devicesSubject: Subject<Device[]> = new Subject();

  private systemConfigUrl = environment.production ? apiURL + 'rest/system/config' : 'api/config';
  private httpOptions;

  private checkInterval: number = 500;

  constructor(private http: HttpClient, private cookieService: CookieService) {
    this.httpOptions = { headers: new HttpHeaders(this.cookieService.getCSRFHeader()) };
  }

  getSystemConfig(): Observable<any> {
    return this.http
      .get(this.systemConfigUrl, this.httpOptions)
      .pipe(map(res => {
        this.systemConfig = res;

        this.folders = res['folders'];
        this.devices = res['devices'];
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
      } else {
        // create timer to keep checking for folders
        let t = setInterval(() => {
          if (check(this.folders))
            clearInterval(t);
          observer.next(this.folders);
        }, this.checkInterval);
      }
    });
    return folderObservable;
  }

  getDevices(): Observable<Device[]> {
    const deviceObserverable: Observable<Device[]> = new Observable((observer) => {
      if (this.folders) {
        observer.next(this.devices);
      } else {
        let t = setInterval(() => {
          if (check(this.devices)) {
            clearInterval(t);
            observer.next(this.devices);
          }
        }, this.checkInterval);
      }
    });
    return deviceObserverable;
  }
}

const check = (target: any): Boolean => {
  if (target) {
    return true;
  }
  return false;
}