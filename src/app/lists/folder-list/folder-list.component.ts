import { AfterViewInit, Component, OnInit, ViewChild, ChangeDetectorRef, OnDestroy } from '@angular/core';
import { MatPaginator } from '@angular/material/paginator';
import { MatSort } from '@angular/material/sort';
import { MatTable, MatTableDataSource } from '@angular/material/table';

import Folder from '../../folder';
import { SystemConfigService } from '../../services/system-config.service';
import { FilterService } from 'src/app/services/filter.service';
import { StType } from 'src/app/type';
import { MatInput } from '@angular/material/input';
import { FolderService } from 'src/app/services/folder.service';
import { trigger, state, style, transition, animate } from '@angular/animations';

@Component({
  selector: 'app-folder-list',
  templateUrl: './folder-list.component.html',
  styleUrls: ['../status-list/status-list.component.scss'],
  animations: [
    trigger('detailExpand', [
      state('collapsed', style({ height: '0px', minHeight: '0' })),
      state('expanded', style({ height: '*' })),
      transition('expanded <=> collapsed', animate('225ms cubic-bezier(0.4, 0.0, 0.2, 1)')),
    ]),
  ],
})
export class FolderListComponent implements AfterViewInit, OnInit, OnDestroy {
  @ViewChild(MatPaginator) paginator: MatPaginator;
  @ViewChild(MatSort) sort: MatSort;
  @ViewChild(MatTable) table: MatTable<Folder>;
  @ViewChild(MatInput) input: MatInput;
  dataSource: MatTableDataSource<Folder>;

  /** Columns displayed in the table. Columns IDs can be added, removed, or reordered. */
  displayedColumns = [
    "id",
    "label",
    "path",
    "state"
  ];

  expandedFolder: Folder | null;

  constructor(
    private folderService: FolderService,
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

    // Replace all data when requests are finished
    this.folderService.foldersUpdated$.subscribe(
      folders => {
        this.dataSource.data = folders;
      }
    );

    // Add device as they come in 
    let folders: Folder[] = [];
    this.folderService.folderAdded$.subscribe(
      folder => {
        folders.push(folder);
        this.dataSource.data = folders;
      }
    );;
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