import { TestBed } from '@angular/core/testing';

import { SystemStatusService } from './system-status.service';
import { HttpClientModule } from '@angular/common/http';

describe('SystemStatusService', () => {
  let service: SystemStatusService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientModule],
      providers: [SystemStatusService]
    });
    service = TestBed.inject(SystemStatusService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
