import { async, ComponentFixture, TestBed } from '@angular/core/testing';

import { FolderListComponent } from './folder-list.component';
import { HttpClientModule } from '@angular/common/http';
import { ChangeDetectorRef } from '@angular/core';

describe('FolderListComponent', () => {
  let component: FolderListComponent;
  let fixture: ComponentFixture<FolderListComponent>;

  beforeEach(async(() => {
    TestBed.configureTestingModule({
      declarations: [FolderListComponent],
      imports: [HttpClientModule],
      providers: [FolderListComponent, ChangeDetectorRef]
    })
      .compileComponents();

    component = TestBed.inject(FolderListComponent);
  }));

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
