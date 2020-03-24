import { Injectable } from '@angular/core';
import { SystemConfigService } from './system-config.service';
import { Observable, Subscriber } from 'rxjs';
import { map } from 'rxjs/operators';
import Folder from './folder';
import { DbStatusService } from './db-status.service';

@Injectable({
  providedIn: 'root'
})
export class FolderService {
  private folders: Folder[];

  constructor(
    private systemConfigService: SystemConfigService,
    private dbStatusService: DbStatusService
  ) { }

  getFolderStatusInOrder(observer: Subscriber<Folder>, startIndex: number) {
    // Return if there aren't any folders at the index
    if (startIndex >= (this.folders.length)) {
      observer.complete();
      return;
    }
    const folder: Folder = this.folders[startIndex];
    startIndex = startIndex + 1;
    this.dbStatusService.getFolderStatus(folder.id).subscribe(
      status => {
        folder.status = status;
        observer.next(folder);

        // recursively get the status of the next folder
        this.getFolderStatusInOrder(observer, startIndex);
      }
    );
  }

  /**
   * getAll() finds all folders and uses db status service to 
   * set all their statuses
   */
  getAll(): Observable<Folder> {
    const folderObservable: Observable<Folder> = new Observable((observer) => {
      this.systemConfigService.getFolders().subscribe(
        folders => {
          this.folders = folders;

          // Synchronously get the status of each folder
          this.getFolderStatusInOrder(observer, 0);
        },
        err => { console.log("getAll error!", err) },
        () => { console.log("get all complete!") }
      );
    });
    return folderObservable
  }
}