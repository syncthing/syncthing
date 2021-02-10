import { TestBed } from '@angular/core/testing';

import { SystemConnectionsService } from './system-connections.service';
import { HttpClientModule } from '@angular/common/http';

describe('SystemConnectionsService', () => {
  let service: SystemConnectionsService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientModule],
      providers: [SystemConnectionsService]
    });
    service = TestBed.inject(SystemConnectionsService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
