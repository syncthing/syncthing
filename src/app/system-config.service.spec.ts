import { TestBed } from '@angular/core/testing';

import { SystemConfigService } from './system-config.service';

describe('SystemConfigService', () => {
  let service: SystemConfigService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(SystemConfigService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
