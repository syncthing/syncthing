import { BrowserModule } from '@angular/platform-browser';
import { NgModule } from '@angular/core';
import { HttpClientModule, HttpClientXsrfModule } from '@angular/common/http';

import { MatTableModule } from '@angular/material/table';
import { MatPaginatorModule } from '@angular/material/paginator';
import { MatSortModule } from '@angular/material/sort';
import { MatButtonToggleModule } from '@angular/material/button-toggle';

import { AppRoutingModule } from './app-routing.module';
import { AppComponent } from './app.component';
import { BrowserAnimationsModule } from '@angular/platform-browser/animations';
import { StatusListComponent } from './status-list/status-list.component';
import { FolderListComponent } from './folder-list/folder-list.component';
import { DeviceListComponent } from './device-list/device-list.component';
import { StatusToggleComponent } from './status-toggle/status-toggle.component';
import { DeviceListDataSource } from './device-list/device-list-datasource';
import { deviceID } from './api-utils';


@NgModule({
  declarations: [
    AppComponent,
    StatusListComponent,
    FolderListComponent,
    DeviceListComponent,
    StatusToggleComponent,
  ],
  imports: [
    BrowserModule,
    AppRoutingModule,
    BrowserAnimationsModule,
    MatTableModule,
    MatPaginatorModule,
    MatSortModule,
    MatButtonToggleModule,
    HttpClientModule,
    HttpClientXsrfModule.withOptions({
      headerName: 'X-CSRF-Token-' + deviceID(),
      cookieName: 'CSRF-Token-' + deviceID(),
    })
  ],
  providers: [],
  bootstrap: [AppComponent]
})

export class AppModule { }


