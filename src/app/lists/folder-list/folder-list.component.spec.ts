import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { FolderListComponent } from './folder-list.component';

describe('FolderListComponent', () => {
  let component: FolderListComponent;
  let fixture: ComponentFixture<FolderListComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [ FolderListComponent ]
    })
    .compileComponents();
  }));

  beforeEach(() => {
    fixture = TestBed.createComponent(FolderListComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
