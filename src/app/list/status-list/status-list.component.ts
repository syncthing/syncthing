import { Component, OnInit } from '@angular/core';
import { Status } from '../../status';
import { cardElevation } from '../../style';


@Component({
  selector: 'app-status-list',
  templateUrl: './status-list.component.html',
  styleUrls: ['./status-list.component.scss']
})
export class StatusListComponent implements OnInit {
  currentStatus: Status = Status.Folders;
  status = Status; // used in html
  elevation: string = cardElevation;

  constructor() { }

  ngOnInit(): void {
  }

  onToggle(s: Status) {
    this.currentStatus = s;
  }
}
