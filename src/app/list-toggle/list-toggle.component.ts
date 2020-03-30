import { Component, EventEmitter, OnInit, Output } from '@angular/core';
import { Type } from '../type';



@Component({
  selector: 'app-list-toggle',
  templateUrl: './list-toggle.component.html',
  styleUrls: ['./list-toggle.component.scss']
})

export class ListToggleComponent implements OnInit {
  public listType = Type;
  @Output() listTypeEvent = new EventEmitter<Type>();

  constructor() { }
  ngOnInit(): void {
  }

  onSelect(t: Type): void {
    this.listTypeEvent.emit(t);
    switch (t) {
      case Type.Folder:
        console.log("folder action");
        break;
      case Type.Device:
        console.log("Device action");
        break;
    }
  }
}