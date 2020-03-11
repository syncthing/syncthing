import { Injectable } from '@angular/core';

import { Observable, of } from 'rxjs';

import { Folder } from './folder';
import { Device } from './device';
import { FOLDERS, DEVICES } from './mock-config-data';

@Injectable({
  providedIn: 'root'
})
export class SystemConfigService {

  constructor() { }

  getFolders(): Observable<Folder[]> {
    return of(FOLDERS);
  }

  getDevices(): Observable<Device[]> {
    return of(DEVICES);
  }

}
