import { Component, OnInit, ViewChild } from '@angular/core';
import Folder from '../../folder'
import { cardElevation } from '../../style'
import { FolderService } from 'src/app/folder.service';
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

  constructor(private folderService: FolderService) { }

  ngOnInit(): void {
    for (let state in Folder.StateType) {
      console.log(state);
    }
  }

  ngAfterViewInit() {
    // TODO: Find total number of folders
    this.folderService.getAll().subscribe(
      folder => {
        // TODO: Clear existing data
        this.donutChart.data([40]);

        // Get StateType and convert to string 
        const stateType: Folder.StateType = Folder.getStateType(folder);
        const state: string = Folder.stateTypeToString(stateType);
        console.log("folder state?", state, folder);
      }
    );

  }
}
