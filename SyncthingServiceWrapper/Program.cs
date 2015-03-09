using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using System.ServiceProcess;
using System.IO;
using System.Reflection;
using System.Diagnostics;

namespace SyncthingServiceWrapper
{
    class WindowsService : ServiceBase
    {
        /// <summary>
        /// Public Constructor for WindowsService.
        /// - Put all of your Initialization code here.
        /// </summary>
        public WindowsService()
        {
            this.ServiceName = "Syncthing Service";
            this.EventLog.Log = "Application";
            this.EventLog.Source = this.ServiceName;

            // These Flags set whether or not to handle that specific
            //  type of event. Set to true if you need it, false otherwise.
            this.CanHandlePowerEvent = true;
            this.CanHandleSessionChangeEvent = true;
            this.CanPauseAndContinue = true;
            this.CanShutdown = true;
            this.CanStop = true;
        }

        /// <summary>
        /// The Main Thread: This is where your Service is Run.
        /// </summary>
        static void Main()
        {
            ServiceBase.Run(new WindowsService());
        }

        /// <summary>
        /// Dispose of objects that need it here.
        /// </summary>
        /// <param name="disposing">Whether
        ///    or not disposing is going on.</param>
        protected override void Dispose(bool disposing)
        {
            syncthingProcess.Dispose();
            base.Dispose(disposing);
        }

        Process syncthingProcess;

        /// <summary>
        /// OnStart(): Put startup code here
        ///  - Start threads, get inital data, etc.
        /// </summary>
        /// <param name="args"></param>
        protected override void OnStart(string[] args)
        {
            syncthingProcess = new Process();
            syncthingProcess.StartInfo.FileName =  Path.Combine(GetCurrentExecutingDirectory(), "syncthing.exe") ;
            syncthingProcess.StartInfo.RedirectStandardOutput = true;
            syncthingProcess.StartInfo.RedirectStandardError = true;
            syncthingProcess.StartInfo.RedirectStandardInput = true;
            syncthingProcess.StartInfo.UseShellExecute = false;
            syncthingProcess.EnableRaisingEvents = true;
            syncthingProcess.StartInfo.CreateNoWindow = false;
            //p.StartInfo.Arguments = concatedParameterAndSources + "\"" + parameter.Destination.LocalPath + "\""; 
            syncthingProcess.OutputDataReceived += new DataReceivedEventHandler(proc_OutputDataReceived);
            syncthingProcess.ErrorDataReceived += new DataReceivedEventHandler(proc_ErrorDataReceived);
            syncthingProcess.Exited += new EventHandler(proc_SyncthingExited);
            syncthingProcess.Start();
            base.OnStart(args);
        }

        private void proc_SyncthingExited(object sender, EventArgs e)
        {
            System.Threading.Thread.Sleep(500);
            var timeout = DateTime.Now;
            while (Process.GetProcessesByName("syncthing.exe").Count() == 0 || DateTime.Compare( timeout.AddMilliseconds(15000), DateTime.Now)>0)
            {
                System.Threading.Thread.Sleep(100); 
            }

            if (Process.GetProcessesByName("syncthing.exe").Count() > 0)
            {
                syncthingProcess = Process.GetProcessesByName("syncthing.exe").Last();
            }
            else
            {
                this.EventLog.WriteEntry("Syncthing.exe crashed");
                this.Stop();
             }
        }

        private void proc_ErrorDataReceived(object sender, DataReceivedEventArgs e)
        {
            this.EventLog.WriteEntry(e.Data);
        }

        private void proc_OutputDataReceived(object sender, DataReceivedEventArgs e)
        {
            this.EventLog.WriteEntry(e.Data);
        }

        private static string GetCurrentExecutingDirectory()
        {
            return Path.GetDirectoryName(new System.Uri(System.Reflection.Assembly.GetExecutingAssembly().CodeBase).LocalPath);
        }

        /// <summary>
        /// OnStop(): Put your stop code here
        /// - Stop threads, set final data, etc.
        /// </summary>
        protected override void OnStop()
        {
            shutdownSyncthingExe();
            base.OnStop();
        }

        private void shutdownSyncthingExe()
        {
            foreach (var item in Process.GetProcessesByName("syncthing.exe"))
            {
                item.StandardInput.WriteLine("\x3");
                item.Close();
                item.Dispose();
            }
            syncthingProcess.Dispose();
        }

        /// <summary>
        /// OnPause: Put your pause code here
        /// - Pause working threads, etc.
        /// </summary>
        protected override void OnPause()
        {

            base.OnPause();
        }

        /// <summary>
        /// OnContinue(): Put your continue code here
        /// - Un-pause working threads, etc.
        /// </summary>
        protected override void OnContinue()
        {
            base.OnContinue();
        }

        /// <summary>
        /// OnShutdown(): Called when the System is shutting down
        /// - Put code here when you need special handling
        ///   of code that deals with a system shutdown, such
        ///   as saving special data before shutdown.
        /// </summary>
        protected override void OnShutdown()
        {
            shutdownSyncthingExe();
            base.OnShutdown();
        }

        /// <summary>
        /// OnCustomCommand(): If you need to send a command to your
        ///   service without the need for Remoting or Sockets, use
        ///   this method to do custom methods.
        /// </summary>
        /// <param name="command">Arbitrary Integer between 128 & 256</param>
        protected override void OnCustomCommand(int command)
        {
            //  A custom command can be sent to a service by using this method:
            //#  int command = 128; //Some Arbitrary number between 128 & 256
            //#  ServiceController sc = new ServiceController("NameOfService");
            //#  sc.ExecuteCommand(command);

            base.OnCustomCommand(command);
        }

        /// <summary>
        /// OnPowerEvent(): Useful for detecting power status changes,
        ///   such as going into Suspend mode or Low Battery for laptops.
        /// </summary>
        /// <param name="powerStatus">The Power Broadcast Status
        /// (BatteryLow, Suspend, etc.)</param>
        protected override bool OnPowerEvent(PowerBroadcastStatus powerStatus)
        {
            return base.OnPowerEvent(powerStatus);
        }

        /// <summary>
        /// OnSessionChange(): To handle a change event
        ///   from a Terminal Server session.
        ///   Useful if you need to determine
        ///   when a user logs in remotely or logs off,
        ///   or when someone logs into the console.
        /// </summary>
        /// <param name="changeDescription">The Session Change
        /// Event that occured.</param>
        protected override void OnSessionChange(
                  SessionChangeDescription changeDescription)
        {
            base.OnSessionChange(changeDescription);
        }
    }
}