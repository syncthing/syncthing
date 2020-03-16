import { AfterViewInit, Component, OnInit, ViewChild } from '@angular/core';
import { MatPaginator } from '@angular/material/paginator';
import { MatSort } from '@angular/material/sort';
import { MatTable } from '@angular/material/table';

import { FolderListDataSource } from './folder-list-datasource';
import { Folder } from '../../folder';
import { SystemConfigService } from '../../system-config.service';


@Component({
  selector: 'app-folder-list',
  templateUrl: './folder-list.component.html',
  styleUrls: ['./folder-list.component.scss']
})
export class FolderListComponent implements AfterViewInit, OnInit {
  @ViewChild(MatPaginator) paginator: MatPaginator;
  @ViewChild(MatSort) sort: MatSort;
  @ViewChild(MatTable) table: MatTable<Folder>;
  dataSource: FolderListDataSource;

  /** Columns displayed in the table. Columns IDs can be added, removed, or reordered. */
  displayedColumns = ['id', 'label'];

  constructor(private systemConfigService: SystemConfigService) { };

  ngOnInit() {
    this.dataSource = new FolderListDataSource(this.systemConfigService);
    this.dataSource.data = [];
  }

  ngAfterContentInit() {
  }

  ngAfterViewInit() {
    this.dataSource.sort = this.sort;
    this.dataSource.paginator = this.paginator;
    this.table.dataSource = this.dataSource;

    this.systemConfigService.getFolders().subscribe(
      data => {
        this.dataSource.data = data;
      }
    );
  }
}
