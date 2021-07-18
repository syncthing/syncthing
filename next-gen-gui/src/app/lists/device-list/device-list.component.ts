import { AfterViewInit, Component, OnInit, ViewChild, ChangeDetectorRef, OnDestroy } from '@angular/core';
import { MatPaginator } from '@angular/material/paginator';
import { MatSort } from '@angular/material/sort';
import { MatTable, MatTableDataSource } from '@angular/material/table';

import Device from '../../device';
import { SystemConfigService } from '../../services/system-config.service';
import { FilterService } from 'src/app/services/filter.service';
import { StType } from 'src/app/type';
import { MatInput } from '@angular/material/input';
import { DeviceService } from 'src/app/services/device.service';
import { trigger, state, style, transition, animate } from '@angular/animations';

@Component({
  selector: 'app-device-list',
  templateUrl: './device-list.component.html',
  styleUrls: ['../status-list/status-list.component.scss'],
  animations: [
    trigger('detailExpand', [
      state('collapsed', style({ height: '0px', minHeight: '0' })),
      state('expanded', style({ height: '*' })),
      transition('expanded <=> collapsed', animate('225ms cubic-bezier(0.4, 0.0, 0.2, 1)')),
    ]),
  ],
})
export class DeviceListComponent implements AfterViewInit, OnInit, OnDestroy {
  @ViewChild(MatPaginator) paginator: MatPaginator;
  @ViewChild(MatSort) sort: MatSort;
  @ViewChild(MatTable) table: MatTable<Device>;
  @ViewChild(MatInput) input: MatInput;
  dataSource: MatTableDataSource<Device>;

  /** Columns displayed in the table. Columns IDs can be added, removed, or reordered. */
  displayedColumns = ['deviceID', 'name', 'state'];
  expandedDevice: Device | null;

  constructor(
    private deviceService: DeviceService,
    private filterService: FilterService,
    private cdr: ChangeDetectorRef,
  ) { };

  applyFilter(event: Event) {
    // Set previous filter value
    const filterValue = (event.target as HTMLInputElement).value;
    this.filterService.previousInputs.set(StType.Device, filterValue);
    this.dataSource.filter = filterValue.trim().toLowerCase();
  }

  ngOnInit() {
    this.dataSource = new MatTableDataSource();
    this.dataSource.data = [];

    // Replace all data when requests are finished
    this.deviceService.devicesUpdated$.subscribe(
      devices => {
        this.dataSource.data = devices;
      }
    );

    // Add device as they come in 
    let devices: Device[] = [];
    this.deviceService.deviceAdded$.subscribe(
      device => {
        devices.push(device);
        this.dataSource.data = devices;
      }
    );
  }

  ngAfterViewInit() {
    this.dataSource.sort = this.sort;
    this.dataSource.paginator = this.paginator;
    this.table.dataSource = this.dataSource;

    const changeText = (text: string) => {
      this.dataSource.filter = text.trim().toLowerCase();
      this.input.value = text;
      this.cdr.detectChanges();
    }

    // Set previous value
    changeText(this.filterService.previousInputs.get(StType.Device));

    // Listen for filter changes from other components
    this.filterService.filterChanged$
      .subscribe(
        input => {
          if (input.type === StType.Device) {
            changeText(input.text);
          }
        });
  }

  ngOnDestroy() { }
}