import { useState } from 'react';
import './App.css';

// Import styles of packages that you've installed.
// All packages except `@mantine/hooks` require styles imports
import '@mantine/core/styles.css';
import '@mantine/dates/styles.css';
import { DateInput } from "@mantine/dates";

import { MantineProvider, Button } from '@mantine/core';

export default function App() {
  const [date, setDate] = useState<Date | null>(null);

  const serverAddress: string = ''

  //sends post request to web server
  async function queryDB() {
      const fetchResponse = await fetch(serverAddress + '/get_runs', {
          method: 'POST',
          headers: {
              Accept: 'application/json',
              'Content-Type': 'application/json'
          }
      })
      console.log(fetchResponse)
      return fetchResponse.status
  }

  return <MantineProvider>
    <DateInput
      value={date}
      onChange={setDate}
      label="Date input"
      placeholder="Date input"
      
    />
    <Button variant="filled" onClick={queryDB}>
      Submit
    </Button>

    </MantineProvider>;
}