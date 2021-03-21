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

  get percentValue(): number {
    let p: number = Math.floor((this.progress / this._total) * 100);
    if (p < 0 || isNaN(p) || p === Infinity) {
      p = 0;
    } else if (p > 100) {
      p = 100;
    }
    return p;
  }

  constructor() { }

  addToProgress(n: number) {
    if (n < 0 || isNaN(n) || n === Infinity) {
      n = 0;
    }

    this.progress += n;
  }

  updateProgress(n: number) {
    if (n < 0 || isNaN(n) || n === Infinity) {
      n = 0
    } else if (n > 100) {
      n = 100
    }

    this.progress = n;
  }

  isComplete(): boolean {
    if (this.progress >= this._total && this.progress > 0 && this._total > 0) {
      return true;
    }

    return false;
  }
}