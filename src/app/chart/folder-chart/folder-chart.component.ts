import { Component, OnInit } from '@angular/core';
import { SystemConfigService } from 'src/app/system-config.service';
import { Folder } from '../../folder'
import { cardElevation } from '../../style'

@Component({
  selector: 'app-folder-chart',
  templateUrl: './folder-chart.component.html',
  styleUrls: ['./folder-chart.component.scss']
})
export class FolderChartComponent implements OnInit {
  chartID: string = 'foldersChart';
  elevation: string = cardElevation;
  data: number[];

  constructor(private systemConfigService: SystemConfigService) { }

  ngOnInit(): void {
    // Find total number of folders
    this.systemConfigService.getFolders().subscribe(
      data => {
        this.data = [0, 1, 32, 40];
      }
    );

    // Sequentially look up each folder to get status
    // dbStatusService
  }
  /*
  ngAfterViewInit() {

  }
  */
}
