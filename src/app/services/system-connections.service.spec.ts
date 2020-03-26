import { TestBed } from '@angular/core/testing';

import { SystemConnectionsService } from './system-connections.service';

describe('SystemConnectionsService', () => {
  let service: SystemConnectionsService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(SystemConnectionsService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
