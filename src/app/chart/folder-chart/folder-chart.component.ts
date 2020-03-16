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
  data: Folder[];

  constructor(private systemConfigService: SystemConfigService) { }

  ngOnInit(): void {
    this.systemConfigService.getFolders().subscribe(
      data => {
        console.log("char folder data", data)
      }
    );
  }

}
