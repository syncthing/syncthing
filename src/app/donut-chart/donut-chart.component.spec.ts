import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { DonutChartComponent } from './donut-chart.component';

describe('DonutChartComponent', () => {
  let component: DonutChartComponent;
  let fixture: ComponentFixture<DonutChartComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ DonutChartComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(DonutChartComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
