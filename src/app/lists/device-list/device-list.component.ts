import { AfterViewInit, Component, OnInit, ViewChild } from '@angular/core';
import { MatPaginator } from '@angular/material/paginator';
import { MatSort } from '@angular/material/sort';
import { MatTable } from '@angular/material/table';

import { DeviceListDataSource } from './device-list-datasource';
import Device from '../../device';
import { SystemConfigService } from '../../services/system-config.service';
import { dataTableElevation } from '../../style';
import { Subject } from 'rxjs';

@Component({
  selector: 'app-device-list',
  templateUrl: './device-list.component.html',
  styleUrls: ['./device-list.component.scss']
})
export class DeviceListComponent implements AfterViewInit, OnInit {
  @ViewChild(MatPaginator) paginator: MatPaginator;
  @ViewChild(MatSort) sort: MatSort;
  @ViewChild(MatTable) table: MatTable<Device>;
  dataSource: DeviceListDataSource;
  elevation: string = dataTableElevation;

  /** Columns displayed in the table. Columns IDs can be added, removed, or reordered. */
  displayedColumns = ['id', 'name'];

  constructor(private systemConfigService: SystemConfigService) { };

  ngOnInit() {
    this.dataSource = new DeviceListDataSource(this.systemConfigService);
    this.dataSource.dataSubject = new Subject<Device[]>()
    this.dataSource.data = [];

    this.systemConfigService.getDevices().subscribe(
      data => {
        this.dataSource.data = data;
        this.dataSource.dataSubject.next(data);
      }
    );
  }

  ngAfterViewInit() {
    this.dataSource.sort = this.sort;
    this.dataSource.paginator = this.paginator;
    this.table.dataSource = this.dataSource;
  }
}