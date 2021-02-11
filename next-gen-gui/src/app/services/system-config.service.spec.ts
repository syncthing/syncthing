import { TestBed } from '@angular/core/testing';

import { SystemConfigService } from './system-config.service';
import { HttpClientModule } from '@angular/common/http';

describe('SystemConfigService', () => {
  let service: SystemConfigService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientModule],
      providers: [SystemConfigService]
    });
    service = TestBed.inject(SystemConfigService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
