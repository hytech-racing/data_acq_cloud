import React, { useState } from 'react';
import logo from './logo.svg';
import './App.css';
import MyDatePicker from './MyDatePicker';

function App() {
  const [mcapDateFiles, setMcapDateFiles] = useState([]);
  const [matDateFiles, setMatDateFiles] = useState([]);
  const serverAddress = 'http://127.0.0.1:5000';
  const [selectedDate, setSelectedDate] = useState(null);

  const handleDateChange = (date) => {
    setSelectedDate(date);
  };
  //handles button click
  const handleButtonClick = async () => {
    if (!selectedDate) {
      console.error('No date selected.');
      return;
    }
  
    // Format selected date into "mm-dd-yyyy" format
    const formattedDate = `${(selectedDate.getMonth() + 1).toString().padStart(2, '0')}-${selectedDate.getDate().toString().padStart(2, '0')}-${selectedDate.getFullYear()}`;

    //Just for testing:
    console.log('Selected date:', formattedDate);
    //http post protocol
    try {
      const response = await fetch(`${serverAddress}/get_runs`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ date: formattedDate }), // date as field
      });
  
      if (!response.ok) {
        throw new Error('Failed to fetch runs');
      }
      //returns all the needed data
      const runsData = await response.json();
  
      const mcapFiles = [];
      const matFiles = [];
      //Put the download links in an arr
      for (let index = 0; index < runsData.length; index++) {
        const element = runsData[index];
        mcapFiles.push(element.mcap_download_link);
        matFiles.push(element.matlab_download_link);
      }
      
      setMcapDateFiles(mcapFiles);
      setMatDateFiles(matFiles);
    } catch (error) {
      console.error('Error fetching runs:', error.message);
    }
  };
  

  return (
    <div className="App">
      <header className="App-header">
        <img src={logo} className="App-logo" alt="logo" />
        <p>
          Edit <code>src/App.js</code> and save to reload.
        </p>
        <a
          className="App-link"
          href="https://reactjs.org"
          target="_blank"
          rel="noopener noreferrer"
        >
          Learn React
        </a>
      </header>
      <MyDatePicker selectedDate={selectedDate} onDateChange={handleDateChange} />
      <button onClick={handleButtonClick}>Get Selected Date</button>

      {/* Render MCAP and MATLAB download links */}
      <div>
        <h3>MCAP Files:</h3>
        {mcapDateFiles.map((link, index) => (
          <a key={index} href={link} target="_blank" rel="noopener noreferrer">{link}</a>
        ))}
      </div>
      <div>
        <h3>MATLAB Files:</h3>
        {matDateFiles.map((link, index) => (
          <a key={index} href={link} target="_blank" rel="noopener noreferrer">{link}</a>
        ))}
      </div>
    </div>
  );
}

export default App;
