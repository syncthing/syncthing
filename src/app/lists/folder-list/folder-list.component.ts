import { AfterViewInit, Component, OnInit, ViewChild, ChangeDetectorRef, OnDestroy } from '@angular/core';
import { MatPaginator } from '@angular/material/paginator';
import { MatSort } from '@angular/material/sort';
import { MatTable, MatTableDataSource } from '@angular/material/table';

import Folder from '../../folder';
import { SystemConfigService } from '../../services/system-config.service';
import { FilterService } from 'src/app/services/filter.service';
import { StType } from 'src/app/type';
import { MatInput } from '@angular/material/input';

@Component({
  selector: 'app-folder-list',
  templateUrl: './folder-list.component.html',
  styleUrls: ['./folder-list.component.scss']
})
export class FolderListComponent implements AfterViewInit, OnInit, OnDestroy {
  @ViewChild(MatPaginator) paginator: MatPaginator;
  @ViewChild(MatSort) sort: MatSort;
  @ViewChild(MatTable) table: MatTable<Folder>;
  @ViewChild(MatInput) input: MatInput;
  dataSource: MatTableDataSource<Folder>;

  /** Columns displayed in the table. Columns IDs can be added, removed, or reordered. */
  displayedColumns = ['id', 'label', 'path', 'state'];

  constructor(
    private systemConfigService: SystemConfigService,
    private filterService: FilterService,
    private cdr: ChangeDetectorRef,
  ) {
  };

  applyFilter(event: Event) {
    const filterValue = (event.target as HTMLInputElement).value;
    this.filterService.previousInputs.set(StType.Folder, filterValue);
    this.dataSource.filter = filterValue.trim().toLowerCase();
  }

  ngOnInit() {
    this.dataSource = new MatTableDataSource();
    this.dataSource.data = [];

    this.systemConfigService.getFolders().subscribe(
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
      this.input.value = text;
      this.cdr.detectChanges();
    }

    // Set previous value
    changeText(this.filterService.previousInputs.get(StType.Folder));

    // Listen for filter changes from other components
    this.filterService.filterChanged$
      .subscribe(
        input => {
          if (input.type === StType.Folder) {
            changeText(input.text);
          }
        });
  }

  ngOnDestroy() { }
}