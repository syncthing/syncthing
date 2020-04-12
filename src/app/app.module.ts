import { BrowserModule } from '@angular/platform-browser';
import { NgModule } from '@angular/core';
import { HttpClientModule, HttpClientXsrfModule } from '@angular/common/http';
import { BrowserAnimationsModule } from '@angular/platform-browser/animations';

import { MatTableModule } from '@angular/material/table';
import { MatPaginatorModule } from '@angular/material/paginator';
import { MatSortModule } from '@angular/material/sort';
import { MatInputModule } from '@angular/material/input';
import { MatButtonToggleModule } from '@angular/material/button-toggle';
import { MatCardModule } from '@angular/material/card';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatDialogModule } from '@angular/material/dialog';
import { MatListModule } from '@angular/material/list'
import { MatButtonModule } from '@angular/material/button';
import { FlexLayoutModule } from '@angular/flex-layout';

import { httpInterceptorProviders } from './http-interceptors';
import { AppRoutingModule } from './app-routing.module';
import { AppComponent } from './app.component';

import { StatusListComponent } from './lists/status-list/status-list.component';
import { DeviceListComponent } from './lists/device-list/device-list.component';
import { DonutChartComponent } from './charts/donut-chart/donut-chart.component';
import { DashboardComponent } from './dashboard/dashboard.component';
import { ListToggleComponent } from './list-toggle/list-toggle.component';

import { HttpClientInMemoryWebApiModule } from 'angular-in-memory-web-api';
import { InMemoryConfigDataService } from './services/in-memory-config-data.service';

import { deviceID } from './api-utils';
import { environment } from '../environments/environment';
import { ChartItemComponent } from './charts/chart-item/chart-item.component';
import { ChartComponent } from './charts/chart/chart.component';
import { FolderListComponent } from './lists/folder-list/folder-list.component';
import { DialogComponent } from './dialog/dialog.component';
import { CardComponent, CardTitleComponent, CardContentComponent } from './card/card.component';
import { TrimPipe } from './trim.pipe';

@NgModule({
  declarations: [
    AppComponent,
    StatusListComponent,
    DeviceListComponent,
    ListToggleComponent,
    DashboardComponent,
    DonutChartComponent,
    ChartComponent,
    ChartItemComponent,
    FolderListComponent,
    DialogComponent,
    CardComponent,
    CardTitleComponent,
    CardContentComponent,
    TrimPipe,
  ],
  imports: [
    BrowserModule,
    AppRoutingModule,
    BrowserAnimationsModule,
    MatInputModule,
    MatTableModule,
    MatPaginatorModule,
    MatSortModule,
    MatButtonToggleModule,
    MatCardModule,
    MatProgressBarModule,
    MatDialogModule,
    MatListModule,
    MatButtonModule,
    FlexLayoutModule,
    HttpClientModule,
    HttpClientXsrfModule.withOptions({
      headerName: 'X-CSRF-Token-' + deviceID(),
      cookieName: 'CSRF-Token-' + deviceID(),
    }),
    environment.production ?
      [] : HttpClientInMemoryWebApiModule.forRoot(InMemoryConfigDataService,
        { dataEncapsulation: false, delay: 10 }),
  ],
  providers: [httpInterceptorProviders],
  bootstrap: [AppComponent]
})

export class AppModule { }


