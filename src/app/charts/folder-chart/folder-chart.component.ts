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
  states: Map<string, number>;
  elevation: string = cardElevation;

  constructor(private folderService: FolderService) {
    this.states = new Map();
  }

  ngOnInit(): void {
    for (let state in Folder.StateType) {
      console.log(state);
    }
  }

  ngAfterViewInit() {
    this.folderService.getAll().subscribe(
      folder => {
        // TODO: Clear existing data
        this.donutChart.data([40]);

        // Get StateType and convert to string 
        const stateType: Folder.StateType = Folder.getStateType(folder);
        const state: string = Folder.stateTypeToString(stateType);

        // Instantiate empty count states
        if (!this.states.has(state)) {
          this.states.set(state, 0);
        }
        const count: number = this.states.get(state) + 1;
        this.states.set(state, count);
      }
    );
  }
}
