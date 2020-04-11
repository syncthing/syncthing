import { Injectable } from '@angular/core';
import { SystemConfigService } from './system-config.service';
import { Observable, Subscriber, Subject, ReplaySubject } from 'rxjs';
import Folder from '../folder';
import { DbStatusService } from './db-status.service';
import { ProgressService } from './progress.service';
import { DbCompletionService } from './db-completion.service';
import { StType } from '../type';

@Injectable({
  providedIn: 'root'
})
export class FolderService {
  private folders: Folder[];
  private foldersSubject: ReplaySubject<Folder[]> = new ReplaySubject(1);
  foldersUpdated$ = this.foldersSubject.asObservable();
  private folderAddedSource = new Subject<Folder>();
  folderAdded$ = this.folderAddedSource.asObservable();

  constructor(
    private systemConfigService: SystemConfigService,
    private dbStatusService: DbStatusService,
    private dbCompletionService: DbCompletionService,
    private progressService: ProgressService,
  ) { }

  getFolderStatusInOrder(startIndex: number) {
    // Return if there aren't any folders at the index
    if (startIndex >= (this.folders.length)) {
      this.foldersSubject.next(this.folders);
      // this.folderAddedSource.complete();
      return;
    }
    const folder: Folder = this.folders[startIndex];
    startIndex = startIndex + 1;
    this.dbStatusService.getFolderStatus(folder.id).subscribe(
      status => {
        folder.status = status;

        this.dbCompletionService.getCompletion(StType.Folder, folder.id).subscribe(
          c => {
            folder.completion = c;
            folder.stateType = Folder.getStateType(folder);
            folder.state = Folder.stateTypeToString(folder.stateType);

            this.folderAddedSource.next(folder);
            this.progressService.addToProgress(1);

            // recursively get the status of the next folder
            this.getFolderStatusInOrder(startIndex);
          });
      }
    );
  }

  /**
   * requestFolders() requests each folder and uses db status service to 
   * set all their statuses and db completion service to find
   * completion in order. Updating folderAdded$ and foldersUpdate$
   * observers
   */
  requestFolders() {
    this.systemConfigService.getFolders().subscribe(
      folders => {
        this.folders = folders;

        // Synchronously get the status of each folder
        this.getFolderStatusInOrder(0);
      }
    );
  }
}