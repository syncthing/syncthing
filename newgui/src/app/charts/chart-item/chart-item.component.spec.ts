import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { ChartItemComponent } from './chart-item.component';

describe('ChartItemComponent', () => {
  let component: ChartItemComponent;
  let fixture: ComponentFixture<ChartItemComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ ChartItemComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(ChartItemComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
