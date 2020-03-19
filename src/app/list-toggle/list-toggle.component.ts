import { Component, EventEmitter, OnInit, Output } from '@angular/core';
import { ListType } from '../list-type';



@Component({
  selector: 'app-list-toggle',
  templateUrl: './list-toggle.component.html',
  styleUrls: ['./list-toggle.component.scss']
})

export class ListToggleComponent implements OnInit {
  public listType = ListType;
  @Output() listTypeEvent = new EventEmitter<ListType>();

  constructor() { }
  ngOnInit(): void {
  }

  onSelect(t: ListType): void {
    this.listTypeEvent.emit(t);
    switch (t) {
      case ListType.Folders:
        console.log("folder action");
        break;
      case ListType.Devices:
        console.log("Device action");
        break;
    }
  }
}