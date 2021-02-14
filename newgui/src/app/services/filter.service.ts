import { Injectable } from '@angular/core';
import { StType } from '../type';
import { Subject } from 'rxjs';

export interface FilterInput {
  type: StType;
  text: string
}

@Injectable({
  providedIn: 'root'
})
export class FilterService {
  previousInputs = new Map<StType, string>(
    [
      [StType.Folder, ""],
      [StType.Device, ""],
    ]
  )

  constructor() { }

  private filterChangeSource = new Subject<FilterInput>();
  filterChanged$ = this.filterChangeSource.asObservable();

  changeFilter(input: FilterInput) {
    this.previousInputs.set(input.type, input.text)
    this.filterChangeSource.next(input);
  }
}
