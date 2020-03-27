import { TestBed } from '@angular/core/testing';

import { SystemStatusService } from './system-status.service';

describe('SystemStatusService', () => {
  let service: SystemStatusService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(SystemStatusService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
