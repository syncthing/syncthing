import { Injectable } from '@angular/core';
import { SystemConfigService } from './system-config.service';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';
import { Folder } from './folder';
import { DbStatusService } from './db-status.service';

@Injectable({
  providedIn: 'root'
})
export class FolderService {

  constructor(
    private systemConfigService: SystemConfigService,
    private dbStatusService: DbStatusService
  ) { }

  /**
   * getAll() finds all folders and uses db status service to 
   * set all their statuses
   */
  getAll(): Observable<Folder> {
    const dbs = this.dbStatusService;
    const folderObservable: Observable<Folder> = new Observable((observer) => {
      this.systemConfigService.getFolders().subscribe({
        next(folders) {
          let folder: Folder;
          for (folder of folders) {
            // Get the status of each folder
            dbs.getFolderStatus(folder.id).subscribe(
              status => {
                console.log(status)
                folder["status"] = status;

                observer.next(folder);
              }
            );
          }
        },
        error(err) { console.log("getAll error!", err) }
      });
    });
    return folderObservable
  }
}
