import { Component, OnInit, ViewChild, Input } from '@angular/core';
import Folder from '../../folder'
import { cardElevation } from '../../style'
import { FolderService } from 'src/app/services/folder.service';
import { DonutChartComponent } from '../donut-chart/donut-chart.component';
import { DeviceService } from 'src/app/services/device.service';
import Device from 'src/app/device';
import { StType } from '../../type';
import { FilterService } from 'src/app/services/filter.service';


export interface ChartItemState {
  label: string,
  count: number,
  color: string,
  selected: boolean,
}
@Component({
  selector: 'app-chart',
  templateUrl: './chart.component.html',
  styleUrls: ['./chart.component.scss']
})

export class ChartComponent implements OnInit {
  @ViewChild(DonutChartComponent) donutChart: DonutChartComponent;
  @Input() type: StType;
  title: string;
  chartID: string;
  states: ChartItemState[] = [];
  elevation: string = cardElevation;

  private service: any;
  private activeChartState: ChartItemState;

  constructor(
    private folderService: FolderService,
    private deviceServce: DeviceService,
    private filterService: FilterService,
  ) { }

  onItemSelect(s: ChartItemState) {
    // Send chart item state to filter
    this.filterService.changeFilter({ type: this.type, text: s.label });

    // Deselect all other items
    this.states.forEach(s => {
      s.selected = false;
    });

    // Select item only
    if (s !== this.activeChartState) {
      s.selected = true;
      this.activeChartState = s;
    } else {
      this.activeChartState = null;
      this.filterService.changeFilter({ type: this.type, text: "" })
    }
  }

  ngOnInit(): void {
    switch (this.type) {
      case StType.Folder:
        this.title = "Folders";
        this.chartID = 'foldersChart';
        this.service = this.folderService;
        break;
      case StType.Device:
        this.title = "Devices";
        this.chartID = 'devicesChart';
        this.service = this.deviceServce;
        break;
    }
  }

  ngAfterViewInit() {
    let totalCount: number = 0;
    this.service.getAll().subscribe(
      t => {
        // Count the number of folders and set chart
        totalCount++;
        this.donutChart.count = totalCount;

        // Get StateType and convert to string 
        const stateType = t.stateType;
        const state = t.state;
        let color;
        switch (this.type) {
          case StType.Folder:
            color = Folder.stateTypeToColor(t.stateType);
            break;
          case StType.Device:
            color = Device.stateTypeToColor(stateType);
            break;
        }

        // Check if state exists
        let found: boolean = false;
        this.states.forEach(s => {
          if (s.label === state) {
            s.count = s.count + 1;
            found = true;
          }
        });

        if (!found) {
          this.states.push({ label: state, count: 1, color: color, selected: false });
        }

        this.donutChart.updateData(this.states);
      },
      err => console.error('Observer got an error: ' + err),
      () => {
      }
    );
  }
}