import { Component, OnInit, ViewChild } from '@angular/core';
import { Folder } from '../../folder'
import { cardElevation } from '../../style'
import { FolderService } from 'src/app/folder.service';
import { SystemConfigService } from 'src/app/system-config.service';
import { DbStatusService } from 'src/app/db-status.service';
import { flatMap } from 'rxjs/operators';
import { DonutChartComponent } from '../donut-chart/donut-chart.component';

@Component({
  selector: 'app-folder-chart',
  templateUrl: './folder-chart.component.html',
  styleUrls: ['./folder-chart.component.scss']
})
export class FolderChartComponent implements OnInit {
  @ViewChild(DonutChartComponent) donutChart: DonutChartComponent;
  chartID: string = 'foldersChart';
  elevation: string = cardElevation;

  constructor(
    private systemConfigService: SystemConfigService,
    private folderService: FolderService,
    private dbStatusService: DbStatusService
  ) { }

  ngOnInit(): void {
  }

  ngAfterViewInit() {
    // TODO: Find total number of folders
    this.folderService.getAll().subscribe(
      folder => {
        // TODO: Clear existing data
        this.donutChart.data([40]);

        console.log("folder?", folder)
      }
    );

  }
}
