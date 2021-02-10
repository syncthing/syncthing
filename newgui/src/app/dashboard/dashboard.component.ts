import { Component, OnInit, AfterViewInit, ViewChild, AfterViewChecked } from '@angular/core';
import {
  trigger,
  state,
  style,
  animate,
  transition,
} from '@angular/animations';
import { SystemConfigService } from '../services/system-config.service';
import { StType } from '../type';
import { FilterService } from '../services/filter.service';
import { ProgressService } from '../services/progress.service';
import { MatProgressBar } from '@angular/material/progress-bar';
import { MessageService } from '../services/message.service';
import { MatDialog, MatDialogRef } from '@angular/material/dialog';
import { DialogComponent } from '../dialog/dialog.component';
import { FolderService } from '../services/folder.service';
import { DeviceService } from '../services/device.service';

@Component({
  selector: 'app-dashboard',
  templateUrl: './dashboard.component.html',
  styleUrls: ['./dashboard.component.scss'],
  providers: [FilterService],
  animations: [
    trigger('loading', [
      state('start', style({
        marginTop: '20px',
      })),
      state('done', style({
        marginTop: '0px',
      })),
      transition('start => done', [
        animate('0.2s 0.2s')
      ]),
      transition('done => start', [
        animate('0.2s 0.2s')
      ]),
    ]),
    trigger('progressBar', [
      state('start', style({
        opacity: 100,
        visibility: 'visible'
      })),
      state('done', style({
        opacity: 0,
        visibility: 'hidden'
      })),
      transition('start => done', [
        animate('0.35s')
      ]),
      transition('done => start', [
        animate('0.35s')
      ]),
    ]),
  ]
})
export class DashboardComponent implements OnInit, AfterViewInit {
  @ViewChild(MatProgressBar) progressBar: MatProgressBar;
  folderChart: StType = StType.Folder;
  deviceChart: StType = StType.Device;
  progressValue: number = 0;
  isLoading = true;
  private dialogRef: MatDialogRef<DialogComponent>;


  constructor(
    private systemConfigService: SystemConfigService,
    private folderService: FolderService,
    private deviceService: DeviceService,
    private progressService: ProgressService,
    private messageService: MessageService,
    public dialog: MatDialog
  ) { }

  ngOnInit() {
    // Request data from Rest API
    this.systemConfigService.getSystemConfig().subscribe(
      _ => {
        // Request devices and folders for charts and lists
        this.folderService.requestFolders();
        this.deviceService.requestDevices();
      }
    );

  }

  ngAfterViewInit() {
    this.isLoading = true;

    // Listen for progress service changes
    let t = setInterval(() => {
      if (this.progressService.isComplete()) {
        clearInterval(t);
        this.progressValue = 100;
        this.isLoading = false;
      }
      this.progressValue = this.progressService.percentValue;
    }, 100);

    // Listen for messages from other services/components
    this.messageService.messageAdded$
      .subscribe(
        _ => {
          // Open dialog
          if (!this.dialogRef)
            this.dialogRef = this.dialog.open(DialogComponent);

          this.dialogRef.afterClosed().subscribe(
            _ => {
              this.dialogRef = null;
              this.messageService.clear();
            }
          );
        });
  }
}