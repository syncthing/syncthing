import { Component, OnInit, ViewChild } from '@angular/core';
import Folder from '../../folder'
import { cardElevation } from '../../style'
import { FolderService } from 'src/app/services/folder.service';
import { DonutChartComponent } from '../donut-chart/donut-chart.component';

@Component({
  selector: 'app-folder-chart',
  templateUrl: './folder-chart.component.html',
  styleUrls: ['./folder-chart.component.scss']
})
export class FolderChartComponent implements OnInit {
  @ViewChild(DonutChartComponent) donutChart: DonutChartComponent;
  chartID: string = 'foldersChart';
  states: { label: string, count: number }[] = [];
  elevation: string = cardElevation;

  constructor(private folderService: FolderService) { }

  ngOnInit(): void {
    // for (let state in Folder.StateType) { }
  }

  ngAfterViewInit() {
    this.folderService.getAll().subscribe(
      folder => {
        // Get StateType and convert to string 
        const stateType: Folder.StateType = Folder.getStateType(folder);
        const state: string = Folder.stateTypeToString(stateType);

        // Check if state exists
        let found: boolean = false;
        this.states.forEach(s => {
          if (s.label === state) {
            s.count = s.count + 1;
            found = true;
          }
        });

        if (!found) {
          this.states.push({ label: state, count: 1 });
        }

        this.donutChart.updateData(this.states);
      },
      err => console.error('Observer got an error: ' + err),
      () => {
      }
    );
  }
}