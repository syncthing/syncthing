import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { FolderChartComponent } from './folder-chart.component';

describe('FolderChartComponent', () => {
  let component: FolderChartComponent;
  let fixture: ComponentFixture<FolderChartComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ FolderChartComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(FolderChartComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
