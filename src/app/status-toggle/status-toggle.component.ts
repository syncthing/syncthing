import { Component, EventEmitter, OnInit, Output } from '@angular/core';
import { Status } from '../status';



@Component({
  selector: 'app-status-toggle',
  templateUrl: './status-toggle.component.html',
  styleUrls: ['./status-toggle.component.scss']
})

export class StatusToggleComponent implements OnInit {
  public status = Status
  @Output() statusEvent = new EventEmitter<Status>();

  constructor() { }
  ngOnInit(): void {
  }

  onSelect(s: Status): void {
    this.statusEvent.emit(s);
    switch (s) {
      case Status.Folders:
        console.log("folder action");
        break;
      case Status.Devices:
        console.log("Device action");
        break;
    }
  }
}