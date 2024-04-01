import React, { useRef, useState } from 'react';
import logo from './logo.svg';
import './App.css';
import MyDatePicker from './MyDatePicker';

function App() {
  const [selectedDate, setSelectedDate] = useState(null);

  const handleDateChange = (date) => {
    setSelectedDate(date);
  };

  const handleButtonClick = () => {
    console.log('Selected date:', selectedDate);
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
    </div>
  );
}

export default App;
