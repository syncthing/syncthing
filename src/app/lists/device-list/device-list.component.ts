import { AfterViewInit, Component, OnInit, ViewChild, ChangeDetectorRef } from '@angular/core';
import { MatPaginator } from '@angular/material/paginator';
import { MatSort } from '@angular/material/sort';
import { MatTable, MatTableDataSource } from '@angular/material/table';

import Device from '../../device';
import { SystemConfigService } from '../../services/system-config.service';
import { FilterService } from 'src/app/services/filter.service';
import { StType } from 'src/app/type';

@Component({
  selector: 'app-device-list',
  templateUrl: './device-list.component.html',
  styleUrls: ['./device-list.component.scss']
})
export class DeviceListComponent implements AfterViewInit, OnInit {
  @ViewChild(MatPaginator) paginator: MatPaginator;
  @ViewChild(MatSort) sort: MatSort;
  @ViewChild(MatTable) table: MatTable<Device>;
  dataSource: MatTableDataSource<Device>;
  filterValue: string = "";

  /** Columns displayed in the table. Columns IDs can be added, removed, or reordered. */
  displayedColumns = ['id', 'name', 'state'];

  constructor(
    private systemConfigService: SystemConfigService,
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

    this.systemConfigService.getDevices().subscribe(
      data => {
        this.dataSource.data = data;
      }
    );
  }

  ngAfterViewInit() {
    this.dataSource.sort = this.sort;
    this.dataSource.paginator = this.paginator;
    this.table.dataSource = this.dataSource;

    const changeText = (text: string) => {
      this.dataSource.filter = text.trim().toLowerCase();
      this.filterValue = text;
      this.cdr.detectChanges();
    }

    // Set previous value
    changeText(this.filterService.previousInputs.get(StType.Device));

    // Listen for filter changes from other components
    this.filterService.filterChanged$.subscribe(
      input => {
        if (input.type === StType.Device) {
          changeText(input.text);
        }
      });
  }
}