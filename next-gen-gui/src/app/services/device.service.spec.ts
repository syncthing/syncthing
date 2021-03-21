import { TestBed } from '@angular/core/testing';

import { DeviceService } from './device.service';
import { HttpClientModule } from '@angular/common/http';

describe('DeviceService', () => {
  let service: DeviceService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientModule],
      providers: [DeviceService]
    });
    service = TestBed.inject(DeviceService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
