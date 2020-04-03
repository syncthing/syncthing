import { Component, OnInit } from '@angular/core';
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

@Component({
  selector: 'app-dashboard',
  templateUrl: './dashboard.component.html',
  styleUrls: ['./dashboard.component.scss'],
  providers: [FilterService],
  animations: [
    trigger('loading', [
      state('start', style({
        marginTop: '100px',
      })),
      state('done', style({
        marginTop: '0px',
      })),
      transition('open => closed', [
        animate('1s')
      ]),
      transition('closed => open', [
        animate('0.5s')
      ]),
    ]),
  ]
})
export class DashboardComponent implements OnInit {
  folderChart: StType = StType.Folder;
  deviceChart: StType = StType.Device;
  isLoading: boolean = true;

  constructor(private systemConfigService: SystemConfigService) { }

  ngOnInit() {
    this.systemConfigService.getSystemConfig().subscribe(
      x => console.log('Observer got a next value: ' + x),
      err => console.error('Observer got an error: ' + err),
      () => console.log('Observer got a complete notification')
    );

    this.isLoading = false;
  }
}
