import { Injectable } from '@angular/core';
import { config } from '../mocks/mock-system-config';
import { dbStatus } from '../mocks/mock-db-status';
import { connections } from '../mocks/mock-system-connections';
import { dbCompletion } from '../mocks/mock-db-completion';
import { systemStatus } from '../mocks/mock-system-status';

@Injectable({
  providedIn: 'root'
})
export class InMemoryConfigDataService {
  createDb() {
    return { config, dbStatus, connections, dbCompletion, systemStatus };
  }

  constructor() { }
}
