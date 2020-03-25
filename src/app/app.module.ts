import { BrowserModule } from '@angular/platform-browser';
import { NgModule } from '@angular/core';
import { HttpClientModule, HttpClientXsrfModule } from '@angular/common/http';

import { MatTableModule } from '@angular/material/table';
import { MatPaginatorModule } from '@angular/material/paginator';
import { MatSortModule } from '@angular/material/sort';
import { MatButtonToggleModule } from '@angular/material/button-toggle';
import { MatCardModule } from '@angular/material/card';
import { FlexLayoutModule } from '@angular/flex-layout';

import { httpInterceptorProviders } from './http-interceptors';
import { AppRoutingModule } from './app-routing.module';
import { AppComponent } from './app.component';

import { BrowserAnimationsModule } from '@angular/platform-browser/animations';
import { StatusListComponent } from './lists/status-list/status-list.component';
import { FolderListComponent } from './lists/folder-list/folder-list.component';
import { DeviceListComponent } from './lists/device-list/device-list.component';
import { DonutChartComponent } from './charts/donut-chart/donut-chart.component';
import { DeviceChartComponent } from './charts/device-chart/device-chart.component';
import { FolderChartComponent } from './charts/folder-chart/folder-chart.component';
import { DashboardComponent } from './dashboard/dashboard.component';
import { ListToggleComponent } from './list-toggle/list-toggle.component';

import { HttpClientInMemoryWebApiModule } from 'angular-in-memory-web-api';
import { InMemoryConfigDataService } from './services/in-memory-config-data.service';

import { deviceID } from './api-utils';
import { environment } from '../environments/environment';
import { ChartItemComponent } from './charts/chart-item/chart-item.component';

@NgModule({
  declarations: [
    AppComponent,
    StatusListComponent,
    FolderListComponent,
    DeviceListComponent,
    ListToggleComponent,
    DashboardComponent,
    DonutChartComponent,
    DeviceChartComponent,
    FolderChartComponent,
    ChartItemComponent,
  ],
  imports: [
    BrowserModule,
    AppRoutingModule,
    BrowserAnimationsModule,
    MatTableModule,
    MatPaginatorModule,
    MatSortModule,
    MatButtonToggleModule,
    MatCardModule,
    FlexLayoutModule,
    HttpClientModule,
    HttpClientXsrfModule.withOptions({
      headerName: 'X-CSRF-Token-' + deviceID(),
      cookieName: 'CSRF-Token-' + deviceID(),
    }),
    environment.production ?
      [] : HttpClientInMemoryWebApiModule.forRoot(InMemoryConfigDataService,
        { dataEncapsulation: false, delay: 200 }),
  ],
  providers: [httpInterceptorProviders],
  bootstrap: [AppComponent]
})

export class AppModule { }


