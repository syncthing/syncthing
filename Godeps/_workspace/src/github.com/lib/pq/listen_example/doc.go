/*

Below you will find a self-contained Go program which uses the LISTEN / NOTIFY
mechanism to avoid polling the database while waiting for more work to arrive.

    //
    // You can see the program in action by defining a function similar to
    // the following:
    //
    // CREATE OR REPLACE FUNCTION public.get_work()
    //   RETURNS bigint
    //   LANGUAGE sql
    //   AS $$
    //     SELECT CASE WHEN random() >= 0.2 THEN int8 '1' END
    //   $$
    // ;

    package main

    import (
        "database/sql"
        "fmt"
        "time"

        "github.com/lib/pq"
    )

    func doWork(db *sql.DB, work int64) {
        // work here
    }

    func getWork(db *sql.DB) {
        for {
            // get work from the database here
            var work sql.NullInt64
            err := db.QueryRow("SELECT get_work()").Scan(&work)
            if err != nil {
                fmt.Println("call to get_work() failed: ", err)
                time.Sleep(10 * time.Second)
                continue
            }
            if !work.Valid {
                // no more work to do
                fmt.Println("ran out of work")
                return
            }

            fmt.Println("starting work on ", work.Int64)
            go doWork(db, work.Int64)
        }
    }

    func waitForNotification(l *pq.Listener) {
        for {
            select {
                case <-l.Notify:
                    fmt.Println("received notification, new work available")
                    return
                case <-time.After(90 * time.Second):
                    go func() {
                        l.Ping()
                    }()
                    // Check if there's more work available, just in case it takes
                    // a while for the Listener to notice connection loss and
                    // reconnect.
                    fmt.Println("received no work for 90 seconds, checking for new work")
                    return
            }
        }
    }

    func main() {
        var conninfo string = ""

        db, err := sql.Open("postgres", conninfo)
        if err != nil {
            panic(err)
        }

        reportProblem := func(ev pq.ListenerEventType, err error) {
            if err != nil {
                fmt.Println(err.Error())
            }
        }

        listener := pq.NewListener(conninfo, 10 * time.Second, time.Minute, reportProblem)
        err = listener.Listen("getwork")
        if err != nil {
            panic(err)
        }

        fmt.Println("entering main loop")
        for {
            // process all available work before waiting for notifications
            getWork(db)
            waitForNotification(listener)
        }
    }


*/
package listen_example
