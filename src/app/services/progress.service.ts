import { Injectable } from '@angular/core';

@Injectable({
  providedIn: 'root'
})
export class ProgressService {
  private progress: number = 0;
  private _total: number = 0;
  set total(t: number) {
    this._total = t;
  }

  get percentValue(): string {
    let p: number = Math.floor((this.progress / this._total) * 100);
    console.log("P?!", NaN)
    if (p < 0 || isNaN(p) || p === Infinity) {
      p = 0;
    } else if (p > 100) {
      p = 100;
    }
    return p.toString();
  }

  constructor() { }

  updateProgress(n: number) {
    if (n < 0) {
      n = 0
    } else if (n > 100) {
      n = 100
    }

    this.progress = n;
  }
}