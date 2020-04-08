import { Injectable } from '@angular/core';
import { Subject } from 'rxjs';

@Injectable({
  providedIn: 'root'
})
export class MessageService {
  messages: string[] = [];
  private messageAddedSource = new Subject<string>();
  messageAdded$ = this.messageAddedSource.asObservable();

  add(message: string) {
    this.messages.push(message);
    this.messageAddedSource.next(message);
  }

  clear() {
    this.messages = [];
  }
}