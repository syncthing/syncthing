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

  constructor(private http: HttpClient, private cookieService: CookieService) {
    this.httpOptions = { headers: new HttpHeaders(this.cookieService.getCSRFHeader()) };
  }

  ngOnInit(): void { }

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
        let checkFolders = (): Boolean => {
          if (this.folders) {
            observer.next(this.folders);
            return true;
          }
          return false;
        }
        let t = setInterval(() => {
          if (checkFolders())
            clearInterval(t);
        }, 100);
      }
    });
    return folderObservable;
  }

  getDevices(): Observable<Device[]> {
    const deviceObserverable: Observable<Device[]> = new Observable((observer) => {
      if (this.folders) {
        observer.next(this.devices);
      } else {
        // create timer to keep checking for devices 
        let checkFolders = (): Boolean => {
          if (this.devices) {
            observer.next(this.devices);
            return true;
          }
          return false;
        }
        let t = setInterval(() => {
          if (checkFolders())
            clearInterval(t);
        }, 100);
      }
    });
    return deviceObserverable;
    // return from(this.devices);
    if (this.devices) {
      this.devicesSubject.next(this.devices);
    }
    return this.devicesSubject;
  }
}