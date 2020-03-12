import { Component, OnInit } from '@angular/core';
import { Status } from '../status';


@Component({
  selector: 'app-status-list',
  templateUrl: './status-list.component.html',
  styleUrls: ['./status-list.component.scss']
})
export class StatusListComponent implements OnInit {
  public currentStatus: Status = Status.Folders;
  public status = Status;

  constructor() { }

  ngOnInit(): void {
  }

  onToggle(s: Status) {
    console.log("holy moly", s);
    this.currentStatus = s;
  }
}
