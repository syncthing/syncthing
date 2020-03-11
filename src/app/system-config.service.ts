import { Injectable } from '@angular/core';

import { Observable, of } from 'rxjs';

import { Folder } from './folder';
import { FOLDERS } from './mock-folders';

@Injectable({
  providedIn: 'root'
})
export class SystemConfigService {

  constructor() { }

  getFolders(): Observable<Folder[]> {
    return of(FOLDERS);
  }
}
