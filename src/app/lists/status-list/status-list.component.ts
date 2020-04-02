import { Component, ViewChild, AfterViewInit, ChangeDetectorRef } from '@angular/core';
import { StType } from '../../type';
import { cardElevation } from '../../style';
import { FilterService } from 'src/app/services/filter.service';
import { ListToggleComponent } from 'src/app/list-toggle/list-toggle.component';


@Component({
  selector: 'app-status-list',
  templateUrl: './status-list.component.html',
  styleUrls: ['./status-list.component.scss']
})
export class StatusListComponent {
  @ViewChild(ListToggleComponent) toggle: ListToggleComponent;
  currentListType: StType = StType.Folder;
  listType = StType; // used in html
  elevation: string = cardElevation;
  title: string = 'Status';

  constructor(
    private filterService: FilterService,
    private cdr: ChangeDetectorRef,
  ) { }

  ngAfterViewInit() {
    // Listen for filter changes from other components
    this.filterService.filterChanged$.subscribe(
      input => {
        this.currentListType = input.type;

        switch (input.type) {
          case StType.Folder:
            this.toggle.group.value = "folders";
            break;
          case StType.Device:
            this.toggle.group.value = "devices";
            break;
        }
      });

    this.cdr.detectChanges(); // manually detect changes
  }

  onToggle(t: StType) {
    this.currentListType = t;
  }
}
