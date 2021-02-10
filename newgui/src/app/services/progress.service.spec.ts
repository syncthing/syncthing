import { TestBed } from '@angular/core/testing';

import { ProgressService } from './progress.service';
import { stringToKeyValue } from '@angular/flex-layout/extended/typings/style/style-transforms';

describe('ProgressService', () => {
  let service: ProgressService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(ProgressService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });

  it('#percentValue should return 0 - 100', () => {
    interface iTest {
      total: number,
      progress: number,
      expected: number,
    }
    const tests: Map<string, iTest> = new Map([
      ["default", { total: 0, progress: 0, expected: 0 }],
      ["NaN return 0", { total: 0, progress: 100, expected: 0 }],
      ["greater than 100 return 100", { total: 10, progress: 100, expected: 100 }],
      ["valid", { total: 100, progress: 100, expected: 100 }],
      ["valid", { total: 100, progress: 50, expected: 50 }],
      ["test floor", { total: 133, progress: 41, expected: 30 }],
    ]);

    service = new ProgressService();
    for (let test of tests.values()) {
      service.total = test.total;
      service.updateProgress(test.progress);
      expect(service.percentValue).toBe(test.expected);
    }
  });
});
