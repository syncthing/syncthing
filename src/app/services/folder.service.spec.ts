import { TestBed } from '@angular/core/testing';

import { FolderService } from './folder.service';
import { HttpClientModule } from '@angular/common/http';

describe('FolderService', () => {
  let service: FolderService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientModule],
      providers: [FolderService]
    });
    service = TestBed.inject(FolderService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
